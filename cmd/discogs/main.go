package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "discogs",
		Short: "Build and maintain a local Discogs SQLite mirror",
	}
	root.AddCommand(fetchCmd(), verifyCmd(), importCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
