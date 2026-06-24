package main

import (
	"fmt"
	"sort"

	"github.com/cehbz/discogs/internal/importer"
	"github.com/spf13/cobra"
)

func importCmd() *cobra.Command {
	var date, dir, out string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Build a fresh SQLite mirror from the dumps in --dir",
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				out = fmt.Sprintf("discogs-%s.db", date)
			}
			rep, err := importer.Import(out, dir, date)
			if err != nil {
				return err
			}
			fmt.Printf("Imported into %s\n", out)
			for _, typ := range []string{"artists", "labels", "masters", "releases"} {
				fmt.Printf("  %-9s %d\n", typ, rep.Counts[typ])
			}
			fmt.Println("Referential integrity (orphan counts):")
			keys := make([]string, 0, len(rep.Integrity.Orphans))
			for k := range rep.Integrity.Orphans {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %-40s %d\n", k, rep.Integrity.Orphans[k])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "dump date YYYYMMDD (required)")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing the dumps")
	cmd.Flags().StringVar(&out, "out", "", "output db path (default discogs-<date>.db)")
	cmd.MarkFlagRequired("date")
	return cmd
}
