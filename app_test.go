package main

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"power-mine/internal/domain"
)

type testWriteCloser struct {
	bytes.Buffer
}

func (w *testWriteCloser) Close() error {
	return nil
}

func TestReserveLaunchBlocksDuplicateUntilReleased(t *testing.T) {
	app := NewApp()

	if err := app.reserveLaunch("profile"); err != nil {
		t.Fatalf("reserveLaunch returned error: %v", err)
	}
	if err := app.reserveLaunch("profile"); err == nil {
		t.Fatal("reserveLaunch should reject duplicate launch")
	}

	app.releaseLaunch("profile")
	if err := app.reserveLaunch("profile"); err != nil {
		t.Fatalf("reserveLaunch after release returned error: %v", err)
	}
}

func TestEnsureLocalServerPortAvailableDetectsConflict(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen on random port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address: %T", listener.Addr())
	}

	err = ensureLocalServerPortAvailable(domain.LocalServer{Port: addr.Port})
	if err == nil {
		t.Fatal("expected occupied port to be rejected")
	}
	want := fmt.Sprintf("local server port %d is already in use", addr.Port)
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("expected error to contain %q, got %q", want, got)
	}
}

func TestLocalServerExitStatusTreatsFastCleanExitAsFailure(t *testing.T) {
	status, message := localServerExitStatus(nil, false, time.Second)
	if status != domain.LaunchFailed {
		t.Fatalf("expected fast clean exit to fail, got %s", status)
	}
	if message != "Local server exited during startup" {
		t.Fatalf("unexpected message: %q", message)
	}

	status, _ = localServerExitStatus(nil, true, time.Second)
	if status != domain.LaunchStopped {
		t.Fatalf("expected requested stop to stay stopped, got %s", status)
	}
}

func TestSendLocalServerStopCommandMarksStopping(t *testing.T) {
	app := NewApp()
	serverID := "server-1"
	runningKey := localServerRunningKey(serverID)
	stdin := &testWriteCloser{}
	app.markLocalServerRunning(runningKey, &exec.Cmd{}, stdin, make(chan struct{}))

	if err := app.sendLocalServerCommand(serverID, " stop\n"); err != nil {
		t.Fatalf("sendLocalServerCommand returned error: %v", err)
	}
	if got := stdin.String(); got != "stop\n" {
		t.Fatalf("unexpected command written to stdin: %q", got)
	}
	if !app.takeLaunchStopping(runningKey) {
		t.Fatal("expected terminal stop command to mark server as stopping")
	}
}

func TestModrinthDependencySelectionMatchesVersionProjectOrFile(t *testing.T) {
	selected := selectedModrinthDependencyMap([]string{"version-1", "project-2", "mod-3.jar"})

	cases := []domain.ModrinthVersion{
		{ID: "version-1"},
		{ProjectID: "project-2"},
		{File: domain.ModrinthVersionFile{FileName: "mod-3.jar"}},
	}
	for _, version := range cases {
		if !modrinthDependencySelected(selected, version) {
			t.Fatalf("expected version to be selected: %#v", version)
		}
	}
	if modrinthDependencySelected(selected, domain.ModrinthVersion{ID: "missing"}) {
		t.Fatal("unexpected selected dependency")
	}
}

func TestModrinthUpdateAvailableComparesVersionThenFile(t *testing.T) {
	installed := domain.ModrinthInstalledProject{
		VersionID: "version-1",
		FileName:  "old.jar",
	}
	if modrinthUpdateAvailable(installed, domain.ModrinthVersion{ID: "version-1", File: domain.ModrinthVersionFile{FileName: "new.jar"}}) {
		t.Fatal("same version id should be up to date")
	}
	if !modrinthUpdateAvailable(installed, domain.ModrinthVersion{ID: "version-2", File: domain.ModrinthVersionFile{FileName: "new.jar"}}) {
		t.Fatal("different version id should need update")
	}
	installed.VersionID = ""
	if !modrinthUpdateAvailable(installed, domain.ModrinthVersion{File: domain.ModrinthVersionFile{FileName: "new.jar"}}) {
		t.Fatal("different file should need update when version id is missing")
	}
}
