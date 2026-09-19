package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/writer"
	"github.com/spf13/cobra"
)

func TestCLIWriterCommandsRejectExistingOwner(t *testing.T) {
	brain := t.TempDir()
	owner, err := writer.AcquireBrain(brain)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	for _, args := range [][]string{{"sync"}, {"extract"}, {"capture"}, {"interview"}, {"inbox"}, {"compact"}, {"cron"}, {"init"}, {"migrate"}, {"import"}, {"config", "set-model"}, {"connectors", "auth"}} {
		cmd := newRootCmd()
		flagRoot = brain
		leaf, _, err := cmd.Find(args)
		if err != nil || leaf.RunE == nil {
			t.Fatalf("missing command %v", args)
		}
		if err := leaf.RunE(leaf, nil); !errors.Is(err, writer.ErrBrainOwned) {
			t.Errorf("%v not fenced: %v", args, err)
		}
	}
	if _, err := os.Stat(filepath.Join(brain, config.FileName)); !os.IsNotExist(err) {
		t.Fatal("contending init modified brain")
	}
}

func TestCLIWriterOwnershipReleasedOnCommandError(t *testing.T) {
	brain := t.TempDir()
	sentinel := errors.New("command failed")
	root := &cobra.Command{Use: "serenity"}
	child := &cobra.Command{Use: "future-writer", RunE: func(*cobra.Command, []string) error { return sentinel }}
	root.AddCommand(child)
	installBrainOwnership(root)
	flagRoot = brain
	if err := child.RunE(child, nil); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	owner, err := writer.AcquireBrain(brain)
	if err != nil {
		t.Fatal("error leaked ownership", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCoexistsWithWriterWithoutWriteAuthority(t *testing.T) {
	brain := t.TempDir()
	if err := config.Default().Save(filepath.Join(brain, config.FileName)); err != nil {
		t.Fatal(err)
	}
	owner, err := writer.AcquireBrain(brain)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"-C", brain, "check", "--actions", "[]", "--json"})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("check failed beside writer: %v", err)
	}
	var verdict struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out.Bytes(), &verdict); err != nil || verdict.Status != "no_applicable_constraints" {
		t.Fatalf("unexpected check verdict: %s", out.String())
	}
}

func TestNoBrainMCPDoesNotCreateRuntimeState(t *testing.T) {
	brain := t.TempDir()
	tools, closeFn, _, _, err := memoryTools(brain, &bytes.Buffer{})
	if err != nil || len(tools) != 0 || closeFn != nil {
		t.Fatalf("transport-only mode changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(brain, ".serenity")); !os.IsNotExist(err) {
		t.Fatal("transport-only mode created runtime state")
	}
}

func TestConnectorsStatusDoesNotAcquireOwnership(t *testing.T) {
	brain := t.TempDir()
	run := func() {
		t.Helper()
		cmd := newRootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-C", brain, "connectors", "status"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if out.Len() == 0 {
			t.Fatal("missing status documentation")
		}
	}
	run()
	if _, err := os.Stat(filepath.Join(brain, ".serenity")); !os.IsNotExist(err) {
		t.Fatal("informational command created runtime state")
	}
	owner, err := writer.AcquireBrain(brain)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	run()
}

func TestServeOwnershipHeldThroughBlockedFlush(t *testing.T) {
	brain := pushFixture(t)
	tools, closeDeps, _, _, err := memoryTools(brain, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	wrote := false
	for _, tool := range tools {
		if tool.Name == "remember" {
			result, err := tool.Handler(context.Background(), json.RawMessage(`{"fact":"flush ownership fixture","provenance":"test"}`))
			if err != nil || result.IsError {
				t.Fatal("remember fixture failed", err)
			}
			wrote = true
		}
	}
	if !wrote {
		t.Fatal("remember not exposed")
	}
	hook := filepath.Join(brain, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch .serenity/flush-ready\nwhile [ ! -e .serenity/flush-release ]; do sleep 0.01; done\n"), 0755); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(brain, ".serenity", "flush-release")
	defer func() { _ = os.WriteFile(release, nil, 0600) }()
	done := make(chan error, 1)
	go func() { done <- closeDeps() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(brain, ".serenity", "flush-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("flush never reached blocking hook")
		}
		time.Sleep(10 * time.Millisecond)
	}
	contender, err := writer.AcquireBrain(brain)
	if contender != nil {
		_ = contender.Close()
	}
	if !errors.Is(err, writer.ErrBrainOwned) {
		t.Fatal("ownership released before flush finished", err)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("flush did not finish")
	}
	owner, err := writer.AcquireBrain(brain)
	if err != nil {
		t.Fatal("completed shutdown retained ownership", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}
