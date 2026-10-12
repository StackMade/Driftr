package installer

// InstallNpm downloads and installs an npm version from the npm registry, to
// run in place of the npm bundled with node.
func InstallNpm(versionStr string, verbose bool) (string, error) {
	return installRegistryTool("npm", "npm@10", versionStr, verbose)
}
