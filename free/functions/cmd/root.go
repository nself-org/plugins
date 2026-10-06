package main

import "github.com/spf13/cobra"

var rootCmd = functionsCmd

func init() {
	rootCmd.Use = "functions"
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true


	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}
