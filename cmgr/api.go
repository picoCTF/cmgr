package cmgr

import (
	"errors"
)

// Creates a new instance of the challenge manager validating the appropriate
// environment variables in the process.  A return value of `nil` indicates
// a fatal error occurred during intitialization.
func NewManager(logLevel LogLevel) *Manager {
	mgr := new(Manager)
	mgr.log = newLogger(logLevel)

	if err := mgr.setChallengeDirectory(); err != nil {
		return nil
	}

	if err := mgr.initDocker(); err != nil {
		return nil
	}

	if err := mgr.initDatabase(); err != nil {
		return nil
	}

	return mgr
}

// Traverses the entire directory and captures all valid challenge descriptions
// it comes across.  In general, it will continue even when it encounters errors
// (permission, poorly formatted JSON, etc.) in order to give the as much
// feedback as possible to the caller.  However, it will fail fast on two
// challenges with the same name and namespace.
//
// This function does not have any side-effects on the database or built/running
// challenge state, but changes that it detects will effect new builds.  It is
// important to resolve any issues/errors it raises before making any other API
// calls for affected challenges.  Failure to follow this guidance could result
// in inconsistencies in deployed challenges.
func (m *Manager) DetectChanges(fp string) *ChallengeUpdates {
	if fp == "" {
		fp = m.chalDir
	}

	cu := new(ChallengeUpdates)

	challenges, errs := m.inventoryChallenges(fp)
	db_metadata, err := m.listChallenges()

	if err != nil {
		cu.Errors = append(errs, err)
		return cu
	}

	for _, curr := range db_metadata {
		newMeta, ok := challenges[curr.Id]
		if !ok {
			cu.Removed = append(cu.Removed, curr)
		} else if curr.Checksum == newMeta.Checksum {
			cu.Unmodified = append(cu.Unmodified, curr)
		} else {
			cu.Updated = append(cu.Updated, newMeta)
		}
		delete(challenges, curr.Id)
	}

	for _, metadata := range challenges {
		cu.Added = append(cu.Added, metadata)
	}

	cu.Errors = errs
	return cu
}

// This challenge will update the global system state based off the changes
// that are detected by a call to `DetectChanges`.  Specifically, in addition
// to updating challenge metadata (new and existing) it will rebuild and, if
// successful restart, existing challenges and then remove the metadata for
// challenges that can no longer be found.  Challenges that have not been
// modified should not be affected.
//
// In the presence of errors, this function will do addition and updates as
// best it can in order to preserve a consistent system state.  However, if
// a build fails, it will keep the existing instance running and rollback the
// challenge metadata.  Additionally, in the presence of errors it will not
// perform any removals of challenge metadata (removing a built challenge is
// considered an error).
func (m *Manager) Update(fp string) *ChallengeUpdates {
	cu := m.DetectChanges(fp)
	errs := m.addChallenges(cu.Added)
	if len(errs) != 0 {
		cu.Errors = append(cu.Errors, errs...)
	}

	errs = m.updateChallenges(cu.Updated)
	if len(errs) != 0 {
		cu.Errors = append(cu.Errors, errs...)
	}

	if len(cu.Errors) == 0 {
		err := m.removeChallenges(cu.Removed)
		if err != nil {
			cu.Errors = append(cu.Errors, err)
		}
	}
	return cu
}

func (m *Manager) Build(challenge ChallengeId, seeds []string, flagFormat string) ([]BuildId, error) {
	return nil, errors.New("`Build` not implemented")
}

func (m *Manager) Start(build BuildId) (InstanceId, error) {
	return 0, errors.New("`Start` not implemented")
}

func (m *Manager) Stop(instance InstanceId) error {
	return errors.New("`Stop` not implemented")
}

func (m *Manager) Destroy(build BuildId) error {
	return errors.New("`Destroy` not implemented")
}

func (m *Manager) CheckInstance(instance InstanceId) (bool, error) {
	return false, errors.New("`CheckInstance` not implemented")
}

func (m *Manager) ListChallenges() []ChallengeId {
	return nil
}

func (m *Manager) GetChallengeMetadata(challenge ChallengeId) (*ChallengeMetadata, error) {
	return nil, errors.New("`GetChallengeMetadata` not implemented")
}

func (m *Manager) GetBuildMetadata(build BuildId) (*BuildMetadata, error) {
	return nil, errors.New("`GetBuildMetadata` not implemented")
}

func (m *Manager) GetInstanceMetadata(instance InstanceId) (*InstanceMetadata, error) {
	return nil, errors.New("`GetInstanceMetadata` not implemented")
}

func (m *Manager) DumpState(challenges []ChallengeId) ([]*ChallengeMetadata, error) {
	return nil, errors.New("`DumpState` not implemented")
}