//go:build darwin || linux

package localruntime

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestNativeFrameBoundariesAndIncompleteInput(t *testing.T) {
	for _, size := range []int{15, 16} {
		input := strings.Repeat("x", size) + "\nnext\n"
		r := bufio.NewReaderSize(strings.NewReader(input), 16)
		frame, err := readNativeFrame(r, 16)
		if err != nil || string(frame) != strings.Repeat("x", size) {
			t.Fatalf("size %d: %q %v", size, frame, err)
		}
		frame, err = readNativeFrame(r, 16)
		if err != nil || string(frame) != "next" {
			t.Fatalf("next frame: %q %v", frame, err)
		}
	}
	if _, err := readNativeFrame(bufio.NewReader(strings.NewReader(strings.Repeat("x", 17)+"\n")), 16); !errors.Is(err, errNativeFrameTooLarge) {
		t.Fatalf("oversized: %v", err)
	}
	if _, err := readNativeFrame(bufio.NewReader(strings.NewReader("partial")), 16); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial: %v", err)
	}
}

func TestNativeFrameAbovePreviousLimitPreservesWire(t *testing.T) {
	wire := []byte(`{"method":"item/started","params":{"data":"` + strings.Repeat("x", 9<<20) + `"}}`)
	input := append(append([]byte(nil), wire...), '\n')
	got, err := readNativeFrame(bufio.NewReaderSize(bytes.NewReader(input), 65536), nativeFrameLimit)
	if err != nil || !bytes.Equal(got, wire) {
		t.Fatalf("large frame changed: %v", err)
	}
	p := &nativeProxy{queries: map[string]string{`"id"`: "thread/list"}}
	if p.nativeDiagnosticMethod(got) != "item/started" || p.nativeDiagnosticMethod([]byte(`{"method":"secret"}`)) != "other_or_response" || p.nativeDiagnosticMethod([]byte(`{"id":"id","result":{}}`)) != "thread/list" {
		t.Fatal("unsafe diagnostic method classification")
	}
}

func TestNativeFrameAcceptsFragmentedInput(t *testing.T) {
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		for _, fragment := range []string{"{\"method\":", "\"turn/started\"}", "\n"} {
			if _, err := io.WriteString(writer, fragment); err != nil {
				done <- err
				return
			}
		}
		done <- writer.Close()
	}()
	frame, err := readNativeFrame(bufio.NewReader(reader), 128)
	if err != nil || string(frame) != `{"method":"turn/started"}` {
		t.Fatalf("fragmented frame: %q %v", frame, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNativeCleanupRequiresEvidence(t *testing.T) {
	if !nativeCleanupVerified(true, false, 0, false, false) || !nativeCleanupVerified(true, false, 0, true, true) {
		t.Fatal("verified exit and command cleanup were not accepted")
	}
	for _, input := range []struct {
		exited, failed bool
		open           int
		ever, ack      bool
	}{{false, false, 0, false, false}, {true, true, 0, false, false}, {true, false, 1, true, true}, {true, false, 0, true, false}} {
		if nativeCleanupVerified(input.exited, input.failed, input.open, input.ever, input.ack) {
			t.Fatalf("accepted incomplete cleanup evidence: %+v", input)
		}
	}
}
