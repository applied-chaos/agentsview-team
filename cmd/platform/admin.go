package main

import (
	"context"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"

	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

// adminCLI runs `platform admin grant|revoke <email>` straight against the database,
// so the first Admin can be made before anyone has signed in.
func adminCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "grant" && args[0] != "revoke") {
		_, _ = fmt.Fprintln(stderr, "usage: platform admin grant|revoke <email>")
		return 2
	}
	email := strings.ToLower(strings.TrimSpace(args[1]))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		_, _ = fmt.Fprintf(stderr, "not an email address: %s\n", args[1])
		return 2
	}
	url := os.Getenv("PLATFORM_DATABASE_URL")
	if url == "" {
		_, _ = fmt.Fprintln(stderr, "PLATFORM_DATABASE_URL is required")
		return 1
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer st.Close()

	now := clock.New().Now()
	if args[0] == "grant" {
		pending, err := st.GrantAdmin(ctx, email, store.CLIActor, now)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		if pending {
			_, _ = fmt.Fprintf(stdout, "%s is pending Admin until first sign-in\n", email)
		} else {
			_, _ = fmt.Fprintf(stdout, "%s is an Admin\n", email)
		}
		return 0
	}
	err = st.RevokeAdmin(ctx, email, store.CLIActor, now)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "%s is not an Admin\n", email)
	return 0
}
