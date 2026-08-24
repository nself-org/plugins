// Purpose: the plugin's cobra root. ProxyCommand in the core CLI's
// internal/plugin/router.go execs this binary as `nself-k8s <args...>` with
// the leading "k8s" argument already stripped, so this root command takes
// the place `nself k8s` used to occupy in the core binary — its subcommands
// (install, upgrade, status) are what `nself k8s install` /
// `nself k8s upgrade` / `nself k8s status` resolve to today.
//
// Inputs: os.Args, as passed through by the plugin router.
//
// Outputs: process exit code (0 success, 1 fail), mirroring the exit codes
// `nself k8s` used before extraction.
//
// Constraints: no dependency on any github.com/nself-org/cli/internal/*
// package — those are unreachable from outside the cli module, and the
// PATH-hijack defence in router.go means this binary must stand on its own.
package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nself-k8s",
	Short: "Manage nSelf on Kubernetes via Helm",
	Long: `Deploy and manage nSelf on any Kubernetes cluster using the official Helm chart.

Requires helm to be installed: https://helm.sh

The official chart is published at https://charts.nself.org.

Examples:
  nself k8s install --domain myapp.com
  nself k8s status
  nself k8s upgrade`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(installCmd, upgradeCmd, statusCmd)
}
