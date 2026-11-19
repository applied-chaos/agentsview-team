package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/applied-chaos/agentsview-team/internal/secrets"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

// secretsCLI runs `platform secrets rotate`: it re-seals every sealed secret from
// PLATFORM_OLD_SECRET_KEY to PLATFORM_SECRET_KEY. Values already on the new key are left
// alone, so a run that stopped part way, or a repeat, is safe. Stop the Platform first, then
// start it with the new key.
func secretsCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "rotate" {
		_, _ = fmt.Fprintln(stderr, "usage: platform secrets rotate (needs PLATFORM_OLD_SECRET_KEY, PLATFORM_SECRET_KEY, PLATFORM_DATABASE_URL)")
		return 2
	}
	oldKey, newKey, url := os.Getenv("PLATFORM_OLD_SECRET_KEY"), os.Getenv("PLATFORM_SECRET_KEY"), os.Getenv("PLATFORM_DATABASE_URL")
	if oldKey == "" || newKey == "" || url == "" {
		_, _ = fmt.Fprintln(stderr, "PLATFORM_OLD_SECRET_KEY, PLATFORM_SECRET_KEY and PLATFORM_DATABASE_URL are required")
		return 1
	}
	oldBox, err1 := secrets.NewBox(oldKey)
	newBox, err2 := secrets.NewBox(newKey)
	if err1 != nil || err2 != nil {
		_, _ = fmt.Fprintln(stderr, "invalid key:", err1, err2)
		return 1
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer st.Close()
	n, err := st.ResealSecrets(ctx, func(label string, sealed []byte) ([]byte, error) {
		if _, err := newBox.Open(label, sealed); err == nil {
			return sealed, nil
		}
		plain, err := oldBox.Open(label, sealed)
		if err != nil {
			return nil, fmt.Errorf("%s opens with neither key: %w", label, err)
		}
		return newBox.Seal(label, plain)
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "re-sealed %d secrets\n", n)
	return 0
}
