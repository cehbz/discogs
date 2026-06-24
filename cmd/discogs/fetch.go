package main

import (
	"context"

	"github.com/cehbz/discogs/internal/dumps"
	"github.com/spf13/cobra"
)

func fetchCmd() *cobra.Command {
	var date, dir, baseURL string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download and verify the four dumps for a given date (YYYYMMDD)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := dumps.Download(context.Background(), baseURL, date, dir, nil); err != nil {
				return err
			}
			return dumps.VerifyChecksums(dir, date)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "dump date YYYYMMDD (required)")
	cmd.Flags().StringVar(&dir, "dir", ".", "output directory")
	cmd.Flags().StringVar(&baseURL, "base-url", dumps.DefaultBaseURL, "dumps base URL")
	cmd.MarkFlagRequired("date")
	return cmd
}
