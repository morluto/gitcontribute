package app

import "context"

type upgradeCommandStub func(context.Context, string, ...string) ([]byte, error)

func (s *Service) stubUpgradeCommand(command upgradeCommandStub) {
	environment := s.upgradeEnv.withDefaults()
	environment.npm = npmUpgradeClient{
		latestVersion: func(ctx context.Context) ([]byte, error) {
			return command(ctx, "npm", "view", "gitcontribute", "version")
		},
		globalRoot: func(ctx context.Context) ([]byte, error) {
			return command(ctx, "npm", "root", "--global")
		},
		install: func(ctx context.Context, version npmVersion) ([]byte, error) {
			return command(ctx, "npm", "install", "--global", "gitcontribute@"+version.String())
		},
	}
	s.upgradeEnv = environment
}

func (s *Service) stubExecutable(executable func() (string, error)) {
	s.executable = executable
}

func (s *Service) stubExecutablePath(path string) {
	s.stubExecutable(func() (string, error) { return path, nil })
}

func (s *Service) stubRuntimeContract(command runtimeContractCommand) {
	environment := s.upgradeEnv.withDefaults()
	environment.runtimeContract = command
	s.upgradeEnv = environment
}

func (s *Service) stubUpgradePlatform(goos string) {
	environment := s.upgradeEnv.withDefaults()
	environment.installPolicy = npmInstallPolicyFor(goos)
	s.upgradeEnv = environment
}
