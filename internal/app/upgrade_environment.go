package app

import (
	"context"
	"os/exec"
	"runtime"

	clientsetup "github.com/morluto/gitcontribute/internal/setup"
)

// npmVersion is a registry-safe release identifier. Construct it at the
// registry or CLI boundary so process execution never receives an unchecked
// package target.
type npmVersion struct {
	value string
}

func parseNPMVersion(value string) (npmVersion, error) {
	resolved, err := clientsetup.ResolveNPMVersion(value)
	if err != nil {
		return npmVersion{}, err
	}
	return npmVersion{value: resolved}, nil
}

func (v npmVersion) String() string {
	return v.value
}

type npmUpgradeClient struct {
	latestVersion func(context.Context) ([]byte, error)
	globalRoot    func(context.Context) ([]byte, error)
	install       func(context.Context, npmVersion) ([]byte, error)
}

type runtimeContractCommand func(context.Context, string) ([]byte, error)

type npmInstallPolicy uint8

const (
	npmInstallPolicyUnset npmInstallPolicy = iota
	npmInstallAutomatic
	npmInstallAfterExit
)

func npmInstallPolicyFor(goos string) npmInstallPolicy {
	if goos == "windows" {
		return npmInstallAfterExit
	}
	return npmInstallAutomatic
}

type upgradeEnvironment struct {
	npm             npmUpgradeClient
	runtimeContract runtimeContractCommand
	installPolicy   npmInstallPolicy
}

func productionUpgradeEnvironment() upgradeEnvironment {
	return upgradeEnvironment{
		npm: npmUpgradeClient{
			latestVersion: func(ctx context.Context) ([]byte, error) {
				return exec.CommandContext(ctx, "npm", "view", "gitcontribute", "version").Output()
			},
			globalRoot: func(ctx context.Context) ([]byte, error) {
				return exec.CommandContext(ctx, "npm", "root", "--global").Output()
			},
			install: func(ctx context.Context, version npmVersion) ([]byte, error) {
				command := exec.CommandContext(ctx, "npm")
				command.Args = []string{"npm", "install", "--global", "gitcontribute@" + version.String()}
				return command.CombinedOutput()
			},
		},
		runtimeContract: func(ctx context.Context, path string) ([]byte, error) {
			return exec.CommandContext(ctx, path, "runtime-contract").CombinedOutput()
		},
		installPolicy: npmInstallPolicyFor(runtime.GOOS),
	}
}

// withDefaults supports deliberately small Service values in package tests
// and library callers while keeping every Upgrade invocation on one immutable
// snapshot of its process capabilities.
func (e upgradeEnvironment) withDefaults() upgradeEnvironment {
	defaults := productionUpgradeEnvironment()
	if e.npm.latestVersion == nil {
		e.npm.latestVersion = defaults.npm.latestVersion
	}
	if e.npm.globalRoot == nil {
		e.npm.globalRoot = defaults.npm.globalRoot
	}
	if e.npm.install == nil {
		e.npm.install = defaults.npm.install
	}
	if e.runtimeContract == nil {
		e.runtimeContract = defaults.runtimeContract
	}
	if e.installPolicy == npmInstallPolicyUnset {
		e.installPolicy = defaults.installPolicy
	}
	return e
}

type upgradeIntent uint8

const (
	upgradeInspect upgradeIntent = iota
	upgradeCheck
	upgradeApply
)

func parseUpgradeIntent(check, apply bool) upgradeIntent {
	if apply {
		return upgradeApply
	}
	if check {
		return upgradeCheck
	}
	return upgradeInspect
}

type installationKind uint8

const (
	installationOther installationKind = iota
	installationNPX
	installationProjectNPM
	installationGlobalNPM
)

func (k installationKind) String() string {
	switch k {
	case installationNPX:
		return "npx"
	case installationProjectNPM:
		return "project-npm"
	case installationGlobalNPM:
		return "global-npm"
	default:
		return "other"
	}
}

func installationKindFromExecutable(executable, globalRoot string) installationKind {
	if executableWithinNPMRoot(executable, globalRoot) {
		return installationGlobalNPM
	}
	return installationProjectNPM
}
