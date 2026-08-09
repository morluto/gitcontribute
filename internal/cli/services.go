package cli

import "github.com/morluto/gitcontribute/internal/contracts"

func serviceOrNotWired[T any](service any) (T, error) {
	typed, ok := service.(T)
	if !ok {
		var zero T
		return zero, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return typed, nil
}

func (c *CLI) discoveryService() (contracts.DiscoveryService, error) {
	return serviceOrNotWired[contracts.DiscoveryService](c.svc)
}
func (c *CLI) tailService() (contracts.TailService, error) {
	return serviceOrNotWired[contracts.TailService](c.svc)
}
func (c *CLI) controlService() (contracts.ControlService, error) {
	return serviceOrNotWired[contracts.ControlService](c.svc)
}
func (c *CLI) workflowService() (contracts.WorkflowService, error) {
	return serviceOrNotWired[contracts.WorkflowService](c.svc)
}
func (c *CLI) dossierService() (contracts.DossierService, error) {
	return serviceOrNotWired[contracts.DossierService](c.svc)
}
func (c *CLI) investigationService() (contracts.InvestigationService, error) {
	return serviceOrNotWired[contracts.InvestigationService](c.svc)
}
func (c *CLI) validationService() (contracts.ValidationService, error) {
	return serviceOrNotWired[contracts.ValidationService](c.svc)
}
func (c *CLI) contributionService() (contracts.ContributionService, error) {
	return serviceOrNotWired[contracts.ContributionService](c.svc)
}
func (c *CLI) clusteringService() (contracts.ClusteringService, error) {
	return serviceOrNotWired[contracts.ClusteringService](c.svc)
}
func (c *CLI) lensService() (contracts.LensService, error) {
	return serviceOrNotWired[contracts.LensService](c.svc)
}
func (c *CLI) collectionService() (contracts.CollectionService, error) {
	return serviceOrNotWired[contracts.CollectionService](c.svc)
}
func (c *CLI) archiveService() (contracts.ArchiveService, error) {
	return serviceOrNotWired[contracts.ArchiveService](c.svc)
}
func (c *CLI) localQueryService() (contracts.LocalQueryService, error) {
	return serviceOrNotWired[contracts.LocalQueryService](c.svc)
}
func (c *CLI) archiveThreadService() (contracts.ArchiveThreadService, error) {
	return serviceOrNotWired[contracts.ArchiveThreadService](c.svc)
}
func (c *CLI) acquisitionService() (contracts.AcquisitionService, error) {
	return serviceOrNotWired[contracts.AcquisitionService](c.svc)
}
func (c *CLI) exportService() (contracts.ExportService, error) {
	return serviceOrNotWired[contracts.ExportService](c.svc)
}
