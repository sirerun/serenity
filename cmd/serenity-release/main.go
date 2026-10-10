// serenity-release is the separately installed operator release authenticator.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/releaseauth"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "release authentication:", err)
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) (runErr error) {
	if len(args) == 0 {
		return fmt.Errorf("command required: canonicalize, sign, verify, prepare, commit, recover")
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(io.Discard)
	p := releaseauth.Paths{LockFD: -1}
	f.StringVar(&p.Archive, "archive", "", "archive path")
	f.StringVar(&p.Manifest, "manifest", "", "manifest path")
	f.StringVar(&p.Signature, "signature", "", "signature path")
	f.StringVar(&p.Policy, "policy", "", "trusted policy path")
	f.StringVar(&p.Approval, "approval", "", "trusted approval path")
	f.StringVar(&p.State, "state", "", "trusted state path")
	f.StringVar(&p.Tag, "tag", "", "expected tag")
	f.IntVar(&p.LockFD, "lock-fd", -1, "inherited deployment lock descriptor")
	var region, credentialsFile, profile, output, installedPin string
	f.StringVar(&region, "region", "", "signing region")
	f.StringVar(&credentialsFile, "credentials-file", "", "explicit credentials file")
	f.StringVar(&profile, "profile", "", "explicit static profile")
	f.StringVar(&output, "output", "", "exclusive signature output")
	f.StringVar(&installedPin, "installed-manifest-sha256", "", "exact recovery identity")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	now := time.Now().UTC()
	switch args[0] {
	case "canonicalize":
		data, err := releaseauth.ReadBounded(p.Manifest, releaseauth.MaxDocument)
		if err != nil {
			return err
		}
		canonical, err := releaseauth.CanonicalManifest(data, now)
		if err != nil {
			return err
		}
		_, err = out.Write(canonical)
		return err
	case "sign":
		if output == "" {
			return fmt.Errorf("signature output required")
		}
		canonical, err := releaseauth.ReadBounded(p.Manifest, releaseauth.MaxDocument)
		if err != nil {
			return err
		}
		policyBytes, err := releaseauth.ReadTrusted(p.Policy)
		if err != nil {
			return err
		}
		var policy releaseauth.Policy
		if err = releaseauth.DecodeCanonical(policyBytes, &policy); err != nil {
			return err
		}
		parts := strings.Split(policy.KeyID, ":")
		if len(parts) != 6 || parts[3] != region {
			return fmt.Errorf("signing region does not match policy key")
		}
		client, err := releaseauth.NewKMSSigner(region, credentialsFile, profile)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		signature, err := releaseauth.Sign(ctx, client, canonical, policy, now)
		if err != nil {
			return err
		}
		dest, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		if _, err = dest.Write(signature); err != nil {
			_ = dest.Close()
			return err
		}
		if err = dest.Sync(); err != nil {
			_ = dest.Close()
			return err
		}
		return dest.Close()
	case "verify", "prepare", "commit", "recover":
		if p.LockFD < -1 {
			return fmt.Errorf("invalid lock descriptor")
		}
		tx, err := releaseauth.OpenTransaction(p)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, tx.Close()) }()
		var result releaseauth.Result
		switch args[0] {
		case "verify":
			result, err = tx.Verify(now)
		case "prepare":
			result, err = tx.Prepare(now)
		case "commit":
			result, err = tx.Commit(now)
		case "recover":
			result, err = tx.Recover(now, installedPin)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(result)
	default:
		return fmt.Errorf("unknown release command")
	}
}
