package installer

// InstallPnpm downloads and installs a pnpm version from the npm registry.
func InstallPnpm(versionStr string, verbose bool) (string, error) {
	return installRegistryTool("pnpm", "pnpm@9", versionStr, verbose)
}
