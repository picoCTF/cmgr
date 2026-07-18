package cmgr

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// solverTimeout bounds how long a solver container may run before the check
// fails; without it a hung solver blocks the caller forever. Generous because
// solvers legitimately compile tooling and brute-force at runtime.
const solverTimeout = 10 * time.Minute

// runSolver builds and runs the challenge's solver against a running instance,
// attaching the solver to that instance's network, and records the solve on
// success.
func (m *Manager) runSolver(instance InstanceId) error {
	iMeta, err := m.lookupInstanceMetadata(instance)
	if err != nil {
		return err
	}

	bMeta, err := m.lookupBuildMetadata(iMeta.Build)
	if err != nil {
		return err
	}

	cMeta, err := m.lookupChallengeMetadata(bMeta.Challenge)
	if err != nil {
		return err
	}

	if !cMeta.SolveScript {
		return fmt.Errorf("no solve script for '%s'", cMeta.Id)
	}

	err = m.executeSolver(cMeta, bMeta, iMeta.getNetworkName())
	if err != nil {
		return err
	}

	iMeta.LastSolved = time.Now().Unix()
	return m.recordSolve(iMeta)
}

// checkBuild builds and runs the challenge's solver against a build that has no
// running instance, on Docker's default network (outbound access, but no
// challenge host), and records the solve against the build on success. This
// supports non-service challenges (artifact-only, flag-only), whose solver
// consumes only the build's outputs and never connects to a challenge host;
// service builds are rejected because their solvers expect a challenge host.
func (m *Manager) checkBuild(cMeta *ChallengeMetadata, bMeta *BuildMetadata) error {
	if cMeta.NeedsInstance() {
		return fmt.Errorf("'%s' is a service challenge: start an instance and check that instead", cMeta.Id)
	}

	if !cMeta.SolveScript {
		return fmt.Errorf("no solve script for '%s'", cMeta.Id)
	}

	err := m.executeSolver(cMeta, bMeta, "")
	if err != nil {
		return err
	}

	bMeta.LastSolved = time.Now().Unix()
	return m.recordBuildSolve(bMeta)
}

// executeSolver builds the solver image for the build, runs it to completion, and
// compares the recovered flag against the build's flag. When netname is non-empty
// the solver is attached to that Docker network (a running instance's network);
// otherwise it runs on Docker's default network with no challenge host. Returns
// nil only when the solver recovers the correct flag; recording the solve is left
// to the caller.
func (m *Manager) executeSolver(cMeta *ChallengeMetadata, bMeta *BuildMetadata, netname string) error {
	// The solver's output is compared against this flag, so an empty flag would
	// let a solver "succeed" by writing an empty file; treat it as an authoring
	// error rather than a solve.
	if bMeta.Flag == "" {
		return fmt.Errorf("build %d of '%s' has an empty flag", bMeta.Id, cMeta.Id)
	}

	solveCtx := m.createSolveContext(cMeta, bMeta)

	imageName := fmt.Sprintf("%s/%s:%d", bMeta.Challenge, "solver", bMeta.Id)
	opts := client.ImageBuildOptions{Remove: true, Tags: []string{imageName}}

	// Build the base image (will run the solver)
	resp, err := m.cli.ImageBuild(m.ctx, solveCtx, opts)
	if err != nil {
		m.log.errorf("failed to build solver image: %s", err)
		return err
	}

	messages, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		m.log.errorf("failed to read build response from docker: %s", err)
		return err
	}

	re := regexp.MustCompile(`{"errorDetail":[^\n]+`)
	errMsg := re.Find(messages)
	if errMsg != nil {
		var dMsg dockerError
		err = json.Unmarshal(errMsg, &dMsg)
		if err == nil {
			errMsg = []byte(dMsg.Error)
		}
		err = fmt.Errorf("failed to build image: %s", errMsg)
		m.log.error(err)
		return err
	}

	iro := client.ImageRemoveOptions{Force: false, PruneChildren: true}
	// Defer the image deletion
	defer m.cli.ImageRemove(m.ctx, imageName, iro)

	// Create a container & run the solver
	cConfig := container.Config{
		Image:    imageName,
		Hostname: "solve",
		Tty:      true,
	}

	hConfig := container.HostConfig{}
	if m.hostOSType == "linux" {
		m.log.debug("inserting custom seccomp profile")
		hConfig.SecurityOpt = []string{"seccomp:" + seccompPolicy}
	}

	// With no instance network (non-service builds) the solver runs on Docker's
	// default bridge: there is no challenge host to reach, but outbound network
	// access is preserved to match the historical behavior of solvers that
	// download tooling or data at runtime.
	nConfig := network.NetworkingConfig{}
	if netname != "" {
		nConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			netname: {
				NetworkID: netname,
				Aliases:   []string{"solver"},
			},
		}
	}

	respCC, err := m.cli.ContainerCreate(m.ctx, client.ContainerCreateOptions{
		Config:           &cConfig,
		HostConfig:       &hConfig,
		NetworkingConfig: &nConfig,
	})
	if err != nil {
		m.log.errorf("failed to create solve container: %s", err)
		return err
	}
	cid := respCC.ID

	cro := client.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	defer m.cli.ContainerRemove(m.ctx, cid, cro)

	_, err = m.cli.ContainerStart(m.ctx, cid, client.ContainerStartOptions{})
	if err != nil {
		m.log.errorf("failed to start solve container: %s", err)
		return err
	}

	// Bound the wait so a hung solver cannot block forever; the deferred
	// force-remove above kills the container if the deadline fires.
	waitCtx, cancel := context.WithTimeout(m.ctx, solverTimeout)
	defer cancel()
	waitRes := m.cli.ContainerWait(waitCtx, cid, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-waitRes.Error:
		if waitCtx.Err() != nil {
			err = fmt.Errorf("solver container did not exit within %s", solverTimeout)
		}
		m.log.errorf("failed to wait on solve container: %s", err)
		return err
	case _ = <-waitRes.Result:
	}

	// Copy out the flag & compare
	res, err := m.cli.CopyFromContainer(m.ctx, cid, client.CopyFromContainerOptions{SourcePath: "/solve/flag"})
	if err != nil {
		m.log.errorf("could not find flag file: %s", err)
		clo := client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		}
		logs, lerr := m.cli.ContainerLogs(m.ctx, cid, clo)
		if lerr != nil {
			m.log.errorf("could not access error logs: %s", lerr)
			err = lerr
		} else {
			s, lerr := ioutil.ReadAll(logs)
			if lerr != nil {
				m.log.errorf("could not read logs: %s", lerr)
				err = lerr
			} else {
				m.log.errorf("logs from failed container: %s", s)
			}
		}

		return err
	}
	flagFileTar := res.Content
	defer flagFileTar.Close()

	fTar := tar.NewReader(flagFileTar)
	for _, err = fTar.Next(); err == nil; _, err = fTar.Next() {
		flag, err := ioutil.ReadAll(fTar)
		if err != nil {
			m.log.errorf("could not read flag file: %s", err)
			return err
		}

		flagStr := strings.TrimSpace(string(flag))
		if flagStr == bMeta.Flag {
			return nil
		}

		return fmt.Errorf("solve script returned incorrect flag: received '%s', expected '%s'", flagStr, bMeta.Flag)
	}

	if err != io.EOF {
		m.log.errorf("error during file copy: %s", err)
		return err
	}

	return errors.New("failed to process flag results properly")
}

func (m *Manager) createSolveContext(cMeta *ChallengeMetadata, meta *BuildMetadata) io.Reader {
	r, w := io.Pipe()
	ctx := tar.NewWriter(w)

	customDocker := false

	go func() {
		// Copy in contents of the "solver" directory
		solveDir := filepath.Join(filepath.Dir(cMeta.Path), "solver")
		err := filepath.Walk(solveDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if path == solveDir {
				return nil
			}

			if path == filepath.Join(solveDir, "Dockerfile") {
				customDocker = true
			}

			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}

			archivePath := path[len(solveDir)+1:]
			hdr.Name = strings.ReplaceAll(archivePath, `\`, `/`)

			err = ctx.WriteHeader(hdr)
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			fd, err := os.Open(path)
			if err != nil {
				return err
			}

			_, err = io.Copy(ctx, fd)
			if err != nil {
				return err
			}
			fd.Close()

			return nil
		})

		if err != nil {
			w.CloseWithError(err)
			return
		}

		if !customDocker {
			// Insert the default
			hdr := tar.Header{Name: "Dockerfile", Mode: 0644, Size: int64(len(m.GetDockerfile("solver")))}
			err = ctx.WriteHeader(&hdr)
			if err != nil {
				w.CloseWithError(err)
				return
			}

			_, err = ctx.Write(m.GetDockerfile("solver"))
			if err != nil {
				w.CloseWithError(err)
				return
			}
		}

		if meta.HasArtifacts {
			artifactsPath := filepath.Join(m.artifactsDir, meta.getArtifactsFilename())
			artifactsFile, err := os.Open(artifactsPath)
			if err != nil {
				w.CloseWithError(err)
				return
			}

			defer artifactsFile.Close()

			artGz, err := gzip.NewReader(artifactsFile)
			if err != nil {
				w.CloseWithError(err)
				return
			}

			artTar := tar.NewReader(artGz)

			// Copy them in
			var h *tar.Header
			for h, err = artTar.Next(); err == nil; h, err = artTar.Next() {
				err = ctx.WriteHeader(h)
				if err != nil {
					w.CloseWithError(err)
					return
				}

				if h.Typeflag != tar.TypeDir {
					_, err = io.Copy(ctx, artTar)
					if err != nil {
						w.CloseWithError(err)
						return
					}
				}
			}

			if err != io.EOF {
				w.CloseWithError(err)
				return
			}

			err = artGz.Close()
			if err != nil {
				w.CloseWithError(err)
				return
			}
		}

		if len(meta.LookupData) > 0 {
			// Create the metadata.json file
			data, err := json.Marshal(meta.LookupData)
			if err != nil {
				w.CloseWithError(err)
				return
			}

			hdr := tar.Header{Name: "metadata.json", Mode: 0644, Size: int64(len(data))}
			err = ctx.WriteHeader(&hdr)
			if err != nil {
				w.CloseWithError(err)
				return
			}

			_, err = ctx.Write(data)
			if err != nil {
				w.CloseWithError(err)
				return
			}
		}

		err = ctx.Close()
		w.CloseWithError(err)
		return
	}()

	return r
}
