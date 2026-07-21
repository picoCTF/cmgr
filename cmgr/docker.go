package cmgr

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	dockeropts "github.com/docker/cli/opts"
	"github.com/docker/go-units"
	"github.com/jmoiron/sqlx"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/strslice"
	"github.com/moby/moby/client"
)

//go:embed seccomp.json
var seccompPolicy string

func (m *Manager) initDocker() error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		m.log.errorf("could not create docker client: %s", err)
		return err
	}

	m.cli = cli
	m.ctx = context.Background()

	var ping client.PingResult
	for attempt := 1; attempt <= 3; attempt++ {
		ping, err = cli.Ping(m.ctx, client.PingOptions{})
		if err == nil {
			break
		}
		if attempt < 3 {
			m.log.warnf("failed to ping docker engine (attempt %d/3): %s. retrying...", attempt, err)
			time.Sleep(1 * time.Second)
		}
	}
	if err != nil {
		m.log.errorf("could not connect to docker engine: %s", err)
		return err
	}

	m.log.infof("connected to docker (API v%s)", ping.APIVersion)

	// OSType is immutable for the daemon's lifetime and is only used to decide
	// whether to apply the linux seccomp profile. Fetch it once here so the hot
	// launch and solve paths never have to call the (heavy) Info endpoint.
	info, err := cli.Info(m.ctx, client.InfoOptions{})
	if err != nil {
		m.log.errorf("could not query docker engine info: %s", err)
		return err
	}
	m.hostOSType = info.Info.OSType

	concurrencyLimit := 2
	if envStr, isSet := os.LookupEnv("CMGR_CONCURRENT_LAUNCHES"); isSet {
		if parsed, err := strconv.Atoi(envStr); err == nil && (parsed == 1 || parsed == 2) {
			concurrencyLimit = parsed
		} else {
			m.log.warnf("invalid CMGR_CONCURRENT_LAUNCHES value '%s' (only 1 or 2 allowed), defaulting to %d", envStr, concurrencyLimit)
		}
	}
	m.log.infof("setting launch concurrency limit to %d", concurrencyLimit)
	m.launchSemaphore = make(chan struct{}, concurrencyLimit)

	chalInterface, isSet := os.LookupEnv(IFACE_ENV)
	if !isSet {
		chalInterface = "0.0.0.0"
	}
	m.challengeInterface = chalInterface

	m.challengeRegistry, isSet = os.LookupEnv(REGISTRY_ENV)
	if isSet {
		authPayload := fmt.Sprintf(
			`{"username":"%s","password":"%s","serveraddress":"%s"}`,
			os.Getenv(REGISTRY_USER_ENV),
			os.Getenv(REGISTRY_TOKEN_ENV),
			strings.SplitN(m.challengeRegistry, "/", 2)[0],
		)
		m.authString = base64.StdEncoding.EncodeToString([]byte(authPayload))
	}

	m.portLow, m.portHigh, err = getPortRange()
	if err != nil {
		m.log.errorf("%s", err)
	}

	return err
}

func getPortRange() (int, int, error) {
	portRange := os.Getenv(PORTS_ENV)
	if portRange == "" {
		return 0, 0, nil
	}

	portStrs := strings.Split(portRange, "-")
	if len(portStrs) != 2 {
		return 0, 0, fmt.Errorf("malformed port range: '%s' does not contain '-' character", portRange)
	}

	var low int
	var high int
	var err error
	low, err = strconv.Atoi(portStrs[0])
	if err == nil {
		high, err = strconv.Atoi(portStrs[1])
	}

	if err != nil {
		return 0, 0, err
	}

	if low < 1024 || high > (1<<16) || high < low {
		err = fmt.Errorf("bad port range: %d-%d either contains invalid/privileged ports or includes 0 ports", low, high)
	}

	return low, high, err
}

func (b *BuildMetadata) makeFlag() *string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", b.Challenge, b.Format, b.Seed)))
	sumStr := fmt.Sprintf("%x", sum)

	flag := new(string)
	if len(sumStr) > 8 {
		sumStr = sumStr[:8]
	}
	*flag = fmt.Sprintf(b.Format, sumStr)
	return flag
}

func (b *BuildMetadata) getArtifactsFilename() string {
	return fmt.Sprintf("%d.tar.gz", b.Id)
}

func (i *InstanceMetadata) getNetworkName() string {
	return fmt.Sprintf("cmgr-%d", i.Id)
}

func (m *Manager) generateBuilds(builds []*BuildMetadata) error {
	if len(builds) == 0 {
		return nil
	}

	buildsComplete := true
	for _, build := range builds {
		buildsComplete = buildsComplete && (build.Flag != "")
	}

	cMeta, err := m.lookupChallengeMetadata(builds[0].Challenge)
	if err != nil {
		return err
	}

	// Note: DetectChanges re-parses and re-hashes the challenge directory on
	// every converge, even when all builds are already complete (below). That
	// cost is inherent to detecting drift here; it is paid per challenge on each
	// CreateSchema/UpdateSchema (e.g. cmgrd instance-count resizes). If it
	// becomes a bottleneck for large schemas, compare only this challenge's
	// stored SourceChecksum instead of running a full cross-schema DetectChanges.
	updates := m.DetectChanges(filepath.Dir(cMeta.Path))

	inList := func(list []*ChallengeMetadata) bool {
		for _, md := range list {
			if md.Id == cMeta.Id {
				return true
			}
		}
		return false
	}
	// The buckets are distinct: only Updated implies changed image content (and
	// hence stale images); Refreshed is a metadata/solve-script change with the
	// source checksum unchanged, so its images are current. modified drives the
	// pending-build error path below (anything not Unmodified is drift).
	sourceChanged := inList(updates.Updated)
	metadataChanged := inList(updates.Refreshed)
	removed := inList(updates.Removed)
	modified := sourceChanged || metadataChanged || removed

	if buildsComplete {
		// Nothing to build, but surface drift instead of returning silently:
		// converging a schema neither rebuilds nor refreshes existing builds —
		// only 'update' does — so name whatever 'update' would act on, without
		// overstating staleness for a metadata-only change.
		switch {
		case len(updates.Errors) > 0:
			m.log.warnf("errors detected in directory for '%s'; run 'update': %v", cMeta.Id, updates.Errors)
		case sourceChanged:
			m.log.warnf("source for '%s' has changed since last update; existing builds and images are stale until 'update' is run", cMeta.Id)
		case metadataChanged:
			m.log.warnf("metadata for '%s' has changed since last update; run 'update' to refresh it (images are unaffected)", cMeta.Id)
		case removed:
			m.log.warnf("source for '%s' can no longer be found; its existing builds are stale", cMeta.Id)
		}
		return nil
	}

	if len(updates.Errors) > 0 {
		err = fmt.Errorf("errors detected in directory for '%s' run 'update'", cMeta.Id)
		m.log.error(err)
		return err
	}

	if modified {
		err = fmt.Errorf("'%s' has changed since last update", cMeta.Id)
		m.log.error(err)
		return err
	}

	buildCtxFile, err := m.createBuildContext(cMeta, m.GetDockerfile(cMeta.ChallengeType))
	if err != nil {
		m.log.errorf("failed to create build context: %s", err)
		return err
	}
	defer os.Remove(buildCtxFile)

	for _, build := range builds {
		if build.Flag != "" {
			continue
		}

		err = m.openBuild(build)
		if err != nil {
			return err
		}

		err = m.executeBuild(cMeta, build, buildCtxFile)
		if err != nil {
			m.removeBuildMetadata(build.Id)
			return err
		}

		err = m.finalizeBuild(build)
		if err != nil {
			return err
		}
	}

	return nil
}

type dockerError struct {
	Error string `json:"error"`
}

// The failure line in a Docker JSON message stream carries both an "error"
// string and an "errorDetail" object; older daemons emitted "errorDetail"
// first while newer ones lead with "error", so match either prefix.
var dockerStreamErrRe = regexp.MustCompile(`{"error[^\n]+`)

// dockerStreamError scans a Docker JSON message stream (from build, push, or
// pull responses, which report failures in-stream rather than as API errors)
// and returns the reported failure, or nil if the stream reports none.
func dockerStreamError(messages []byte) error {
	errMsg := dockerStreamErrRe.Find(messages)
	if errMsg == nil {
		return nil
	}
	var dMsg dockerError
	if json.Unmarshal(errMsg, &dMsg) == nil && dMsg.Error != "" {
		return errors.New(dMsg.Error)
	}
	return errors.New(string(errMsg))
}

// contentChecksum derives the content identity of a build's images from the
// challenge source checksum and the flag format. The source content reaches
// the image through the build context (and the frozen base image); the flag
// format enters as a build arg. The seed also affects image content but is
// carried explicitly in the docker tag, so it is not folded in here.
//
// Caveat: the embedded per-type Dockerfile (m.GetDockerfile) is part of the
// build context but NOT part of this checksum. So a rebuild whose only change
// is the challenge type, or a cmgr release shipping a different built-in
// Dockerfile (e.g. a base-image bump), yields different image content under an
// unchanged tag. Within one database this is just the tag reuse the scheme
// already tolerates; across databases sharing a daemon it means identical
// tuples can resolve to differing content. Folding the type Dockerfile (or a
// build-machinery version) in here would close that, but the migration
// backfill cannot reconstruct a legacy row's historical Dockerfile, so it is
// deferred.
func contentChecksum(sourceChecksum uint32, format string) uint32 {
	h := crc32.NewIEEE()
	var src [4]byte
	binary.BigEndian.PutUint32(src[:], sourceChecksum)
	h.Write(src[:])
	h.Write([]byte(format))
	return h.Sum32()
}

// dockerId is the docker tag for one of the build's images. It is derived
// from portable build identity — seed plus content checksum — rather than the
// local autoincrement build id, so the same challenge content yields the same
// tag no matter which cmgr database built it, and a source change yields a
// new tag instead of mutating the old one. The "s" prefix keeps the tag valid
// when the seed is negative.
func (bMeta *BuildMetadata) dockerId(image Image) string {
	return fmt.Sprintf("s%d-%x-%s", bMeta.Seed, bMeta.Checksum, image.Host)
}

// migrateBuildChecksums backfills builds.checksum for rows still at the default
// value (0) and retags their local docker images from the legacy {buildid}-{host}
// tag form to the content-addressed form (see dockerId). The backfill derives
// each build's checksum from its challenge's current source checksum, which is
// what the images were built from as of the last update — the same assumption
// the pre-checksum code made when it reused a tag across rebuilds.
//
// It is resumable and idempotent: the work is keyed on checksum=0 (the
// "not yet migrated" marker) rather than on the column's existence, and each
// row's images are retagged BEFORE its checksum is stamped, so an interruption
// or transient docker error leaves the row at 0 to be retried on the next
// start. Called from initDatabase before m.db is assigned, so the handle is
// passed in explicitly; m.cli is nil when the database is initialized without
// docker (tests), in which case retagging is skipped.
func (m *Manager) migrateBuildChecksums(db *sqlx.DB) error {
	rows := []struct {
		Id             BuildId
		Seed           int
		Format         string
		Challenge      string
		SourceChecksum uint32 `db:"sourcechecksum"`
	}{}
	err := db.Select(&rows, `SELECT b.id, b.seed, b.format, b.challenge, c.sourcechecksum
		FROM builds AS b JOIN challenges AS c ON b.challenge = c.id
		WHERE b.checksum = 0;`)
	if err != nil {
		return err
	}

	for _, row := range rows {
		checksum := contentChecksum(row.SourceChecksum, row.Format)

		// Retag first: checksum=0 is the resume marker, so it is stamped only
		// once the images provably carry the new tag (or are shown to need no
		// retag). A genuine docker error aborts, leaving the row at 0.
		if err := m.retagLegacyImages(db, row.Id, row.Challenge, row.Seed, checksum); err != nil {
			return err
		}

		if _, err := db.Exec("UPDATE builds SET checksum = ? WHERE id = ?;", checksum, row.Id); err != nil {
			return err
		}
	}

	// Rows that never joined a challenge (e.g. out-of-band DB edits with foreign
	// keys off) stay at checksum=0 and would yield an invalid s{seed}-0 tag;
	// surface them rather than failing silently at launch time.
	var unmigrated int
	if err := db.Get(&unmigrated, "SELECT COUNT(1) FROM builds WHERE checksum = 0;"); err != nil {
		return err
	}
	if unmigrated > 0 {
		m.log.warnf("%d build(s) have no content checksum (missing challenge row?); their images cannot be resolved until the challenge is rebuilt", unmigrated)
	}

	return nil
}

// retagLegacyImages moves a build's images from the legacy {challenge}:{id}-{host}
// tag form to the content-addressed {challenge}:s{seed}-{checksum}-{host} form.
// It is idempotent and tolerant of a missing source image (already retagged on
// a prior interrupted run, or a fresh daemon that will rebuild on demand): only
// a genuine daemon error is returned, so migrateBuildChecksums can defer
// stamping the checksum until the retag has succeeded or been shown
// unnecessary. Skipped without a docker client (tests).
func (m *Manager) retagLegacyImages(db *sqlx.DB, id BuildId, challenge string, seed int, checksum uint32) error {
	if m.cli == nil {
		return nil
	}

	hosts := []string{}
	if err := db.Select(&hosts, "SELECT host FROM images WHERE build = ?;", id); err != nil {
		return err
	}

	newMeta := BuildMetadata{Seed: seed, Checksum: checksum}
	for _, host := range hosts {
		oldRef := fmt.Sprintf("%s:%d-%s", challenge, id, host)
		newRef := fmt.Sprintf("%s:%s", challenge, newMeta.dockerId(Image{Host: host}))

		if _, err := m.cli.ImageTag(m.ctx, client.ImageTagOptions{Source: oldRef, Target: newRef}); err != nil {
			if !errdefs.IsNotFound(err) {
				// A real daemon error, not a missing image: abort so the row
				// stays at checksum=0 and the retag is retried next start.
				return fmt.Errorf("could not retag legacy image %s as %s: %w", oldRef, newRef, err)
			}
			// Source absent: either already retagged (new ref present) or never
			// present locally (fresh daemon / pruned out-of-band). Nothing to
			// recover; the next rebuild recreates it under the new name.
			m.log.debugf("legacy image %s not present; skipping retag", oldRef)
			continue
		}
		// Drop the legacy ref; the image survives under the new one. A missing
		// legacy tag here is fine (idempotent re-run after a prior removal).
		if _, err := m.cli.ImageRemove(m.ctx, oldRef, client.ImageRemoveOptions{Force: false, PruneChildren: false}); err != nil && !errdefs.IsNotFound(err) {
			m.log.warnf("could not remove legacy image tag %s: %s", oldRef, err)
		}
		m.log.infof("retagged image %s as %s", oldRef, newRef)
	}
	return nil
}

func challengeToFreezeName(challenge ChallengeId) string {
	return strings.ReplaceAll(string(challenge), "/", "_")
}

func (m *Manager) freezeBaseImage(challenge ChallengeId, force bool) error {
	cMeta, err := m.lookupChallengeMetadata(challenge)
	if err != nil {
		return err
	}

	imageName := fmt.Sprintf("%s/%s:%x", m.challengeRegistry, challengeToFreezeName(challenge), cMeta.SourceChecksum)

	if !force {
		// Do some check here to see if it already exists
	}

	buildCtxFile, err := m.createBuildContext(cMeta, m.GetDockerfile(cMeta.ChallengeType))
	if err != nil {
		m.log.errorf("failed to create build context: %s", err)
		return err
	}
	defer os.Remove(buildCtxFile)
	buildCtx, err := os.Open(buildCtxFile)
	if err != nil {
		m.log.errorf("failed to seek to beginning of file for %s: %s", cMeta.Id, err)
		return err
	}
	defer buildCtx.Close()

	// Setup build options
	opts := client.ImageBuildOptions{
		Remove:     true,
		Tags:       []string{imageName},
		Target:     "base",
		NoCache:    force, // Require to use latest info on force
		PullParent: force, // Update parent image as well on force
		Labels: map[string]string{
			"cmgr.managed":   "true",
			"cmgr.challenge": string(challenge),
		},
	}

	// Build the image
	m.log.debugf("creating base image %s", imageName)
	resp, err := m.cli.ImageBuild(m.ctx, buildCtx, opts)
	if err != nil {
		m.log.errorf("failed to build base image: %s", err)
		return err
	}

	// Read the response because errors aren't propagated.
	messages, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		m.log.errorf("failed to read build response from docker: %s", err)
		return err
	}

	// Search the response for an error message
	if streamErr := dockerStreamError(messages); streamErr != nil {
		err = fmt.Errorf("failed to build image: %s", streamErr)
		m.log.error(err)
		return err
	}

	// Push the image
	pushOpts := client.ImagePushOptions{RegistryAuth: m.authString}
	pushResp, err := m.cli.ImagePush(m.ctx, imageName, pushOpts)
	if err != nil {
		m.log.errorf("failed to push base image: %s", err)
		return err
	}

	// Read the response because errors aren't propagated.
	messages, err = ioutil.ReadAll(pushResp)
	pushResp.Close()
	if err != nil {
		m.log.errorf("failed to read push response from docker: %s", err)
		return err
	}

	// Search the response for an error message
	if streamErr := dockerStreamError(messages); streamErr != nil {
		err = fmt.Errorf("failed to push image: %s", streamErr)
		m.log.error(err)
		return err
	}

	return nil
}

func (m *Manager) executeBuild(cMeta *ChallengeMetadata, bMeta *BuildMetadata, buildCtxFile string) error {

	seedStr := fmt.Sprintf("%d", bMeta.Seed)

	// Stamp the build with the content identity its images are about to be
	// produced from; dockerId (and therefore every tag below) depends on it.
	bMeta.Checksum = contentChecksum(cMeta.SourceChecksum, bMeta.Format)

	baseName := fmt.Sprintf("%s/%s:%x", m.challengeRegistry, challengeToFreezeName(cMeta.Id), cMeta.SourceChecksum)
	pullOpts := client.ImagePullOptions{RegistryAuth: m.authString}
	var buildCache []string
	pullResp, err := m.cli.ImagePull(m.ctx, baseName, pullOpts)
	if err == nil {
		// Read the response because errors aren't propagated.
		messages, err := ioutil.ReadAll(pullResp)
		pullResp.Close()
		if err == nil {
			// Search the response for an error message
			if dockerStreamError(messages) == nil {
				m.log.infof("Successfully pulled base image '%s'", baseName)
				buildCache = append(buildCache, baseName)
			}
		}
	}

	images := []Image{}
	var buildImage string
	for _, host := range cMeta.Hosts {
		image := Image{Host: host.Name, Ports: []string{}}
		imageName := fmt.Sprintf("%s:%s", cMeta.Id, bMeta.dockerId(image))

		if host.Name == "builder" || (host.Name == "challenge" && buildImage == "") {
			buildImage = imageName
		}

		for _, portInfo := range cMeta.PortMap {
			if portInfo.Host == image.Host {
				image.Ports = append(image.Ports, fmt.Sprintf("%d/tcp", portInfo.Port))
			}
		}

		// Setup build options
		opts := client.ImageBuildOptions{
			BuildArgs: map[string]*string{
				"FLAG_FORMAT": &bMeta.Format,
				"SEED":        &seedStr,
				"FLAG":        bMeta.makeFlag(),
			},
			Remove:    true,
			CacheFrom: buildCache,
			Tags:      []string{imageName},
			Target:    host.Target,
			Labels: map[string]string{
				"cmgr.managed":   "true",
				"cmgr.challenge": string(cMeta.Id),
			},
		}

		// Call build
		buildCtx, err := os.Open(buildCtxFile)
		if err != nil {
			m.log.errorf("failed to seek to beginning of file for %s/%d: %s", cMeta.Id, bMeta.Id, err)
			return err
		}
		defer buildCtx.Close()

		m.log.debugf("creating image %s", imageName)
		resp, err := m.cli.ImageBuild(m.ctx, buildCtx, opts)
		if err != nil {
			m.log.errorf("failed to build base image: %s", err)
			return err
		}

		messages, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			m.log.errorf("failed to read build response from docker: %s", err)
			return err
		}

		if streamErr := dockerStreamError(messages); streamErr != nil {
			err = fmt.Errorf("failed to build image: %s", streamErr)
			m.log.error(err)
			return err
		}
		images = append(images, image)
	}

	if buildImage == "" {
		err := fmt.Errorf("aborting because no build image identified %s/%d", cMeta.Id, bMeta.Id)
		m.log.error(err)
		return err
	}

	// This container is created only to copy the built /challenge tree out of the
	// image; it is never started. Override the command so creation succeeds even
	// for images with no CMD/ENTRYPOINT (e.g. artifact-only custom Dockerfiles),
	// which Docker otherwise rejects with "no command specified".
	// NOTE: a future `rollback` would re-run everything from here down against
	// the PrevChecksum image (no docker build) to restore that generation's
	// artifacts, lookups, and DB state.
	cConfig := container.Config{Image: buildImage, Cmd: []string{"true"}}
	hConfig := container.HostConfig{}
	nConfig := network.NetworkingConfig{}

	if m.hostOSType == "linux" {
		m.log.debug("inserting custom seccomp profile")
		hConfig.SecurityOpt = []string{"seccomp:" + seccompPolicy}
	}

	respCC, err := m.cli.ContainerCreate(m.ctx, client.ContainerCreateOptions{
		Config:           &cConfig,
		HostConfig:       &hConfig,
		NetworkingConfig: &nConfig,
	})
	if err != nil {
		m.log.errorf("failed to create artifacts container: %s", err)
		return err
	}

	cid := respCC.ID
	crOpts := client.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	defer m.cli.ContainerRemove(m.ctx, cid, crOpts)

	m.log.infof("created container %s", cid)

	res, err := m.cli.CopyFromContainer(m.ctx, cid, client.CopyFromContainerOptions{SourcePath: "/challenge"})
	if err != nil {
		m.log.errorf("could not find '/challenge' in container: %s", err)
		return err
	}
	metaFile := res.Content
	defer metaFile.Close()

	cTar := tar.NewReader(metaFile)
	var hdr *tar.Header
	var lookups map[string]string
	var files []string
	var flag string
	for hdr, err = cTar.Next(); err == nil; hdr, err = cTar.Next() {
		m.log.debugf("found in tar: %s", hdr.Name)
		if hdr.Name == "challenge/metadata.json" {
			data, err := ioutil.ReadAll(cTar)
			if err != nil {
				m.log.errorf("could not read metadata.json: %s", err)
				return err
			}

			lookups = make(map[string]string)
			err = json.Unmarshal(data, &lookups)
			if err != nil {
				m.log.errorf("could not decode build metadata JSON file: %s", err)
				return err
			}

			var ok bool
			flag, ok = lookups["flag"]
			if !ok {
				err = errors.New("build metadata missing the flag")
				m.log.error(err)
				return err
			}

			delete(lookups, "flag")
		} else if hdr.Name == "challenge/artifacts.tar.gz" {
			artifactsFileName := bMeta.getArtifactsFilename()
			// Iterate through reading filenames and copying over the tarball
			artifactsFile, err := os.Create(filepath.Join(m.artifactsDir, artifactsFileName))
			if err != nil {
				m.log.errorf("could not create cached artifacts archive: %s", err)
				return err
			}
			defer artifactsFile.Close()

			srcGz, err := gzip.NewReader(cTar)
			if err != nil {
				m.log.errorf("could not gzip read artifacts file: %s", err)
				return err
			}

			dstGz := gzip.NewWriter(artifactsFile)
			srcTar := tar.NewReader(srcGz)
			dstTar := tar.NewWriter(dstGz)

			var h *tar.Header
			for h, err = srcTar.Next(); err == nil; h, err = srcTar.Next() {
				files = append(files, h.Name)
				m.log.debugf("artifact found: %s", h.Name)
				err = dstTar.WriteHeader(h)
				if err != nil {
					m.log.errorf("could not write header to artifacts file: %s", err)
					return err
				}

				if h.Typeflag != tar.TypeDir {
					_, err = io.Copy(dstTar, srcTar)
					if err != nil {
						m.log.errorf("could not write body to artifacts file: %s", err)
						return err
					}
				}
			}

			if err != io.EOF {
				m.log.errorf("error occurred during copy of artifacts: %s", err)
				return err
			}

			err = dstTar.Close()
			if err != nil {
				m.log.errorf("error closing artifacts tar file: %s", err)
				return err
			}

			err = srcGz.Close()
			if err != nil {
				m.log.errorf("error closing GZIP decoder: %s", err)
				return err
			}

			err = dstGz.Close()
			if err != nil {
				m.log.errorf("error closing GZIP encoder: %s", err)
				return err
			}

			err = artifactsFile.Close()
			if err != nil {
				m.log.errorf("error occurred when closing artifacts: %s", err)
				return err
			}
		}
	}

	if err != io.EOF {
		m.log.errorf("could not read metadata file: %s", err)
		return err
	}

	if flag == "" {
		err = errors.New("'flag' missing in metadata.json")
		m.log.error(err)
		return err
	}

	bMeta.Flag = flag
	bMeta.LookupData = lookups
	bMeta.Images = images
	bMeta.HasArtifacts = len(files) > 0

	err = m.validateBuild(cMeta, bMeta, files)
	if err != nil {
		os.Remove(bMeta.getArtifactsFilename())

		// Free the build image now rather than waiting for the deferred
		// container cleanup: while the extraction container exists it references
		// the image, so the non-forced ImageRemove below could not untag it.
		// (The deferred removal then no-ops on the already-gone container.)
		_, _ = m.cli.ContainerRemove(m.ctx, cid, crOpts)

		// Untag the failed generation's images unless they must survive: another
		// build row may share this exact (challenge, seed, format, checksum)
		// tuple (e.g. the same challenge and seed in two schemas), or THIS row —
		// on the rebuild path, not yet finalized — may retain this checksum as
		// its rollback target (PrevChecksum), as after an A->B->A source revert.
		// Both cases keep the images; only a genuinely unreferenced failed
		// generation is removed. imageMu keeps the check-and-remove atomic
		// against concurrent removers (destroyImages/pruneReplacedImages).
		m.imageMu.Lock()
		if bMeta.PrevChecksum != bMeta.Checksum && !m.contentReferenced(bMeta, bMeta.Id) {
			iro := client.ImageRemoveOptions{Force: false, PruneChildren: true}
			for _, image := range bMeta.Images {
				imageName := fmt.Sprintf("%s:%s", bMeta.Challenge, bMeta.dockerId(image))
				_, _ = m.cli.ImageRemove(m.ctx, imageName, iro)
			}
		}
		m.imageMu.Unlock()
	}

	m.log.debugf("%v", bMeta)

	return err
}

func (m *Manager) startNetwork(instance *InstanceMetadata, opts NetworkOptions) error {
	netSpec := client.NetworkCreateOptions{
		Driver: "bridge",
	}
	netname := instance.getNetworkName()
	_, err := m.cli.NetworkCreate(m.ctx, netname, netSpec)
	if err != nil {
		m.log.errorf("could not create challenge network (%s): %s", netname, err)
	}
	return err
}

func (m *Manager) stopNetwork(instance *InstanceMetadata) error {
	networkName := instance.getNetworkName()
	_, err := m.cli.NetworkRemove(m.ctx, networkName, client.NetworkRemoveOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			m.log.warnf("skipped removing network (not found): %s", networkName)
			err = nil
		} else {
			m.log.errorf("failed to remove network: %s", err)
		}
	}
	return err
}

// portsAlreadyKnown reports whether every published port for an image already has
// a non-zero host port reserved in ports, so the post-start read-back in
// startContainers can be safely skipped. It is false when explicit ports are
// disabled (portLow == 0, Docker assigns ephemeral ports) or when any required
// port is unassigned — e.g. the rebuild flow, which clears instance.Ports before
// restarting and so must read the bound port back. A port whose name is missing
// from revPortMap is treated as unknown (false): without a resolvable name we
// cannot confirm its reservation, and must not skip the read-back.
func portsAlreadyKnown(portLow int, imagePorts []string, ports map[string]int, revPortMap map[string]string) bool {
	if portLow == 0 {
		return false
	}
	for _, portStr := range imagePorts {
		name, ok := revPortMap[portStr]
		if !ok || name == "" || ports[name] == 0 {
			return false
		}
	}
	return true
}

func (m *Manager) startContainers(build *BuildMetadata, instance *InstanceMetadata, opts map[string]ContainerOptions, envVars map[string]string, revPortMap map[string]string) error {
	m.launchSemaphore <- struct{}{}
	defer func() { <-m.launchSemaphore }()

	// Call create in docker
	netname := instance.getNetworkName()
	for _, image := range build.Images {
		if image.Host == "builder" {
			continue
		}
		exposedPorts := network.PortSet{}
		publishedPorts := network.PortMap{}
		for _, portStr := range image.Ports {
			port, err := network.ParsePort(portStr)
			if err != nil {
				return fmt.Errorf("invalid port %q in image configuration: %w", portStr, err)
			}
			var hostPort string
			if m.portLow == 0 {
				hostPort = ""
			} else {
				hostPort = strconv.Itoa(instance.Ports[revPortMap[portStr]])
			}

			exposedPorts[port] = struct{}{}
			var addr netip.Addr
			if m.challengeInterface != "" {
				addr, err = netip.ParseAddr(m.challengeInterface)
				if err != nil {
					return fmt.Errorf("invalid challenge interface %q: %w", m.challengeInterface, err)
				}
			}
			publishedPorts[port] = []network.PortBinding{
				{HostIP: addr, HostPort: hostPort},
			}
		}

		isDynamicInstance := "false"
		if build.InstanceCount == DYNAMIC_INSTANCES {
			isDynamicInstance = "true"
		}

		cLabels := map[string]string{
			"cmgr.managed": "true",
			"cmgr.dynamic": isDynamicInstance,
		}

		cConfig := container.Config{
			Image:        fmt.Sprintf("%s:%s", build.Challenge, build.dockerId(image)),
			Hostname:     image.Host,
			ExposedPorts: exposedPorts,
			Labels:       cLabels,
		}

		// Note: envVars (including user_id and any caller-supplied variables) are
		// injected identically into every container in the build. In multi-container
		// challenges all containers will receive the same set of environment variables.
		if len(envVars) > 0 {
			var envList []string
			for k, v := range envVars {
				envList = append(envList, fmt.Sprintf("%s=%s", k, v))
			}
			cConfig.Env = append(cConfig.Env, envList...)
		}

		hConfig := container.HostConfig{
			PortBindings:  publishedPorts,
			RestartPolicy: container.RestartPolicy{Name: "always"},
		}

		hasContainerOpts := false
		cOpts, hasContainerOpts := opts[""]
		if hostCOpts, ok := opts[strings.ToLower(image.Host)]; ok {
			cOpts = hostCOpts
			hasContainerOpts = true
		}
		if image.Host == "builder" {
			hasContainerOpts = false
		}
		if hasContainerOpts {
			hConfig.Init = &cOpts.Init
			if cOpts.Cpus != "" {
				nanoCpus, err := dockeropts.ParseCPUs(cOpts.Cpus)
				if err != nil {
					return err
				}
				hConfig.NanoCPUs = nanoCpus
			}
			if cOpts.Memory != "" {
				memoryBytes, err := units.RAMInBytes(cOpts.Memory)
				if err != nil {
					return err
				}
				hConfig.Memory = memoryBytes
			}
			if len(cOpts.Ulimits) > 0 {
				limits := make([]*units.Ulimit, len(cOpts.Ulimits))
				for i, limitStr := range cOpts.Ulimits {
					limit, err := units.ParseUlimit(limitStr)
					if err != nil {
						return err
					}
					limits[i] = limit
				}
				hConfig.Ulimits = limits
			}
			if cOpts.PidsLimit != 0 {
				hConfig.PidsLimit = &cOpts.PidsLimit
			}
			hConfig.ReadonlyRootfs = cOpts.ReadonlyRootfs
			hConfig.CapDrop = (strslice.StrSlice)(cOpts.DroppedCaps)
			if cOpts.NoNewPrivileges {
				hConfig.SecurityOpt = append(hConfig.SecurityOpt, "no-new-privileges:true")
			}
			if cOpts.DiskQuota != "" {
				_, quotas_enabled := os.LookupEnv(DISK_QUOTA_ENV)
				if quotas_enabled {
					var storageOpt = map[string]string{
						"size": cOpts.DiskQuota,
					}
					hConfig.StorageOpt = storageOpt
				} else {
					m.log.warnf("disk quota for %s container '%s' ignored (disk quotas are not enabled)", build.Challenge, image.Host)
				}
			}
			if cOpts.CgroupParent != "" {
				hConfig.CgroupParent = cOpts.CgroupParent
			}
		}

		if m.hostOSType == "linux" {
			if hasContainerOpts && cOpts.CapImmutable {
				hConfig.CapAdd = append(hConfig.CapAdd, "LINUX_IMMUTABLE")
			}
			m.log.debug("inserting custom seccomp profile")
			hConfig.SecurityOpt = append(hConfig.SecurityOpt, "seccomp:"+seccompPolicy)
		}

		nConfig := network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				netname: {
					NetworkID: netname,
					Aliases:   []string{image.Host},
				},
			},
		}

		respCC, err := m.cli.ContainerCreate(m.ctx, client.ContainerCreateOptions{
			Config:           &cConfig,
			HostConfig:       &hConfig,
			NetworkingConfig: &nConfig,
		})
		if err != nil {
			m.log.errorf("failed to create instance container: %s", err)
			return err
		}

		cid := respCC.ID
		instance.Containers = append(instance.Containers, cid)
		m.log.infof("created new container: %s", cid)

		_, err = m.cli.ContainerStart(m.ctx, cid, client.ContainerStartOptions{})
		if err != nil {
			m.log.errorf("failed to start container: %s", err)
			return err
		}

		// When the port mapping is already known (explicit ports that were reserved
		// and bound before ContainerCreate), instance.Ports already holds the correct
		// values and a successful ContainerStart guarantees the port is bound. The
		// inspect/backoff loop below could only re-read the value we already set (and
		// would needlessly eat backoff sleeps while networking settles), so skip it.
		// It is still needed when the mapping is not yet known: Docker-assigned
		// ephemeral ports, or an explicit-port path entered with instance.Ports
		// cleared (e.g. rebuild), where the bound port must be read back.
		if !portsAlreadyKnown(m.portLow, image.Ports, instance.Ports, revPortMap) {
			backoff := time.Millisecond
			done := false
			for !done && backoff < time.Second {
				m.log.debug("Querying docker for port info...")

				cInfo, err := m.cli.ContainerInspect(m.ctx, cid, client.ContainerInspectOptions{})
				if err != nil {
					m.log.errorf("failed to get container info: %s", err)
					return err
				}
				if cInfo.Container.NetworkSettings == nil {
					done = false
					time.Sleep(backoff)
					backoff = 2 * backoff
					continue
				}

				done = true
				for cPort, hPortInfo := range cInfo.Container.NetworkSettings.Ports {
					if len(hPortInfo) == 0 {
						done = false
						time.Sleep(backoff)
						backoff = 2 * backoff
						break
					}

					hPort, err := strconv.Atoi(string(hPortInfo[0].HostPort))
					if err != nil {
						return err
					}
					name, ok := revPortMap[cPort.String()]
					if !ok {
						// No reverse-map entry: writing under "" would drop the real
						// mapping (and clobber other unmapped ports). Skip it instead.
						m.log.warnf("ignoring container port %s with no reverse-port-map entry", cPort)
						continue
					}
					instance.Ports[name] = hPort
					m.log.debugf("container port %s mapped to %s", cPort, hPortInfo[0].HostPort)
				}
			}
		}
	}

	return m.finalizeInstance(instance)
}

func (m *Manager) stopContainers(instance *InstanceMetadata) error {
	var err error
	for _, cid := range instance.Containers {
		crOpts := client.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
		_, err = m.cli.ContainerRemove(m.ctx, cid, crOpts)
		if err != nil {
			if errdefs.IsNotFound(err) {
				m.log.warnf("skipped removing container (not found): %s", cid)
				err = nil
			} else {
				m.log.errorf("failed to remove container: %s", err)
			}
		}
	}

	mdErr := m.removeContainersMetadata(instance)
	if mdErr != nil {
		err = mdErr
	}

	return err
}

// replacedImages records the docker tags of a build generation superseded by
// a rebuild, along with the identity tuple those tags encode.
type replacedImages struct {
	tags []string
	meta BuildMetadata
}

// pruneReplacedImages untags image generations that fell out of retention
// after an update rebuild (the 'update --prune-old' flow): the generation
// displaced from PrevChecksum, never the newly retained rollback target. Each
// entry is skipped while any live build row still resolves to its tuple;
// removal failures are logged but never fatal — a leaked tag is recoverable,
// a failed update is not.
func (m *Manager) pruneReplacedImages(replaced []replacedImages) {
	// Serialize the reference-check-then-remove against concurrent removers
	// (destroyImages, executeBuild cleanup): content-addressed tags are shared
	// across build rows, so an unsynchronized check could untag images a build
	// finalized between the check and the removal.
	m.imageMu.Lock()
	defer m.imageMu.Unlock()

	iro := client.ImageRemoveOptions{Force: false, PruneChildren: true}
	for _, r := range replaced {
		if m.contentReferenced(&r.meta, 0) {
			m.log.debugf("keeping replaced images for %s: content still referenced by another build", r.meta.Challenge)
			continue
		}
		for _, tag := range r.tags {
			if _, err := m.cli.ImageRemove(m.ctx, tag, iro); err != nil {
				if errdefs.IsNotFound(err) {
					// Already removed — e.g. a build in another schema shared
					// the tuple and its entry was pruned first.
					m.log.debugf("replaced image already gone: %s", tag)
				} else {
					m.log.warnf("could not prune replaced image %s: %s", tag, err)
				}
			} else {
				m.log.infof("pruned replaced image %s", tag)
			}
		}
	}
}

func (m *Manager) destroyImages(build BuildId) error {
	m.log.debugf("destroying build %d", build)
	bMeta, err := m.lookupBuildMetadata(build)
	if err != nil {
		return err
	}

	err = m.removeBuildMetadata(build)
	if err != nil {
		return err
	}

	if bMeta.HasArtifacts {
		artifactsFilename := bMeta.getArtifactsFilename()
		err := os.Remove(filepath.Join(m.artifactsDir, artifactsFilename))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				m.log.warnf("skipped removing artifacts file (not found): %s", artifactsFilename)
				err = nil
			} else {
				m.log.errorf("failed to remove artifacts file: %s", err)
				return err
			}
		}
	}

	// The build's metadata row is already gone, so any remaining row holding
	// this content as its current or rollback generation is another build that
	// shares these exact tags (e.g. the same challenge and seed in two
	// schemas); in that case the images must survive. The current and rollback
	// generations are checked independently — either can outlive the other.
	//
	// Force is false and removal failures are non-fatal by design: a content
	// tag can be referenced outside this cmgr database — another cmgr instance
	// on the same daemon that built identical content, possibly with a running
	// or stopped container this database cannot see. Refusing to delete an
	// in-use image and leaking the tag is recoverable; force-removing it out
	// from under another instance is not. This mirrors the opt-in rationale for
	// 'update --prune-old' (see UpdateOptions.PruneOldImages). imageMu serializes
	// the reference-check-then-remove against concurrent removers.
	m.imageMu.Lock()
	defer m.imageMu.Unlock()

	iro := client.ImageRemoveOptions{Force: false, PruneChildren: true}
	if m.contentReferenced(bMeta, bMeta.Id) {
		m.log.debugf("keeping images for destroyed build %d: content still referenced by another build", build)
	} else {
		for _, image := range bMeta.Images {
			imageName := fmt.Sprintf("%s:%s", bMeta.Challenge, bMeta.dockerId(image))
			if _, err := m.cli.ImageRemove(m.ctx, imageName, iro); err != nil {
				if errdefs.IsNotFound(err) {
					m.log.warnf("skipped removing image (not found): %s", imageName)
				} else {
					m.log.warnf("could not remove image %s (leaving it in place): %s", imageName, err)
				}
			}
		}
	}

	// Retire the build's retained rollback generation the same way; its tags
	// may already be gone (never rotated, or pruned via another row), so
	// removal here is best-effort.
	if bMeta.PrevChecksum != 0 && bMeta.PrevChecksum != bMeta.Checksum {
		prevMeta := BuildMetadata{
			Challenge: bMeta.Challenge,
			Seed:      bMeta.Seed,
			Format:    bMeta.Format,
			Checksum:  bMeta.PrevChecksum,
		}
		if !m.contentReferenced(&prevMeta, bMeta.Id) {
			for _, image := range bMeta.Images {
				imageName := fmt.Sprintf("%s:%s", bMeta.Challenge, prevMeta.dockerId(image))
				if _, err := m.cli.ImageRemove(m.ctx, imageName, iro); err != nil && !errdefs.IsNotFound(err) {
					m.log.warnf("could not remove rollback-generation image %s: %s", imageName, err)
				}
			}
		}
	}

	return nil
}
