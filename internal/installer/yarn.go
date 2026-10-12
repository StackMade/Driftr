package installer

// InstallYarn downloads and installs a yarn version from the npm registry.
func InstallYarn(versionStr string, verbose bool) (string, error) {
	return installRegistryTool("yarn", "yarn@1", versionStr, verbose)
}
