package main

import (
	"github.com/cehbz/discogs/internal/dumps"
	"github.com/spf13/cobra"
)

func verifyCmd() *cobra.Command {
	var date, dir string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify SHA-256 checksums of already-downloaded dumps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dumps.VerifyChecksums(dir, date)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "dump date YYYYMMDD (required)")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing the dumps")
	cmd.MarkFlagRequired("date")
	return cmd
}
