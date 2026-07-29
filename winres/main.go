// Command winres compiles the Windows resources for nimbus.exe into a COFF
// object that the Go linker picks up automatically: an application manifest,
// a VERSIONINFO block and the application icon.
//
// It exists because the alternatives do not fit this project:
//
//   - windres (binutils) needs a mingw-w64 cross toolchain installed, i.e. a C
//     toolchain in the build path of a project that deliberately builds with
//     CGO_ENABLED=0. It is also not present on this machine.
//   - rsrc (github.com/akavel/rsrc) is pure Go and runs anywhere, but it only
//     emits an icon and a manifest - it has no VERSIONINFO support at all.
//     It is what produced the old checked-in nimbus.syso.
//   - goversioninfo (github.com/josephspurrier/goversioninfo) is pure Go and
//     does all three, but its CLI hands out resource IDs in a fixed order: with
//     a manifest present the icon group lands on ID 2, while internal/ui calls
//     LoadIcon(hInst, MAKEINTRESOURCE(1)) and would get NULL. Since v1.7.0 it
//     also writes the icon a second time under IDI_APPLICATION, which for this
//     project's 279 KB .ico means a 560 KB object.
//
// So this program uses goversioninfo and rsrc as libraries and does the
// resource-ID assignment itself. It is pure Go, needs no cgo and no Windows,
// and produces a byte-for-byte reproducible object: no timestamps are written.
//
// Resource layout produced (verify any build with -dump):
//
//	RT_ICON       (3)  id 2..N   one per frame in the .ico
//	RT_GROUP_ICON (14) id 1      what LoadIcon(MAKEINTRESOURCE(1)) resolves,
//	                             and the lowest group ID, so Explorer shows it
//	                             as the file icon
//	RT_VERSION    (16) id 1      VS_VERSIONINFO, the Properties dialog
//	RT_MANIFEST   (24) id 1      CREATEPROCESS_MANIFEST_RESOURCE_ID
//
// Usage:
//
//	go run . -version 1.0.15 -out ../nimbus_windows_amd64.syso
//	go run . -dump ../nimbus_windows_amd64.syso
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"

	"github.com/akavel/rsrc/binutil"
	"github.com/akavel/rsrc/coff"
	"github.com/akavel/rsrc/ico"
	"github.com/josephspurrier/goversioninfo"
)

// Resource types, from winuser.h. rsrc/coff only names three of them.
const (
	rtIcon      = uint32(3)
	rtGroupIcon = uint32(14)
	rtVersion   = uint32(16)
	rtManifest  = uint32(24)
)

const (
	// CREATEPROCESS_MANIFEST_RESOURCE_ID. Windows looks for the process
	// activation context under exactly this ID in an .exe; ID 2 is for a DLL
	// and ID 3 for an isolation-aware one. Getting this wrong means the
	// manifest is embedded but silently ignored.
	manifestID = uint16(1)

	// internal/ui/{settings,forecast,about}_windows.go all register their
	// window class with LoadIcon(d.inst, win.MAKEINTRESOURCE(1)). That call
	// looks up RT_GROUP_ICON by integer name, so the group has to be 1.
	groupIconID = uint16(1)

	// GetFileVersionInfo on an .exe reads RT_VERSION 1 and nothing else.
	versionID = uint16(1)

	// RT_ICON IDs start after the group so the numbering matches what rsrc
	// produced for the previous nimbus.syso. The two namespaces are separate,
	// so this is cosmetic - but a diff of the old and new object should show
	// only additions.
	firstIconID = uint16(2)
)

func main() {
	dump := flag.String("dump", "", "instead of generating, dump the resource tree of an existing .syso and exit")
	icon := flag.String("icon", "../nimbusicon.ico", "path to the application .ico")
	manifest := flag.String("manifest", "nimbus.exe.manifest", "path to the application manifest XML")
	viJSON := flag.String("versioninfo", "versioninfo.json", "path to the VERSIONINFO source JSON")
	version := flag.String("version", "", "version to stamp, e.g. 1.0.15 (required unless -dump)")
	arch := flag.String("arch", "amd64", "target architecture: 386, amd64, arm or arm64")
	out := flag.String("out", "../nimbus_windows_amd64.syso", "output .syso path")
	flag.Parse()

	if *dump != "" {
		if err := dumpSyso(*dump, os.Stdout); err != nil {
			fatal(err)
		}
		return
	}
	if *version == "" {
		fatal(fmt.Errorf("-version is required; run `make resource` rather than this program directly"))
	}
	if err := generate(*icon, *manifest, *viJSON, *version, *arch, *out); err != nil {
		fatal(err)
	}
}

func generate(iconPath, manifestPath, viPath, version, arch, outPath string) error {
	vi, err := loadVersionInfo(viPath, version)
	if err != nil {
		return err
	}

	// Build() fills the VS_VERSIONINFO struct tree from the parsed config,
	// Walk() serialises it into vi.Buffer. Both are required, in that order,
	// before the buffer holds anything.
	vi.Build()
	vi.Walk()

	rsrc := coff.NewRSRC()
	if err := rsrc.Arch(arch); err != nil {
		return err
	}

	// coff.AddResource sorts the type level on insert but appends blindly at
	// the ID level, so IDs must be added in ascending order within a type.
	// The PE resource directory is binary-searched by the loader, so an
	// unsorted directory is a resource that cannot be found at runtime.
	iconFile, err := os.Open(iconPath)
	if err != nil {
		return err
	}
	defer iconFile.Close()

	frames, err := ico.DecodeHeaders(iconFile)
	if err != nil {
		return fmt.Errorf("%s: %w", iconPath, err)
	}
	if len(frames) == 0 {
		return fmt.Errorf("%s: contains no icon frames", iconPath)
	}

	group := grpIconDir{ICONDIR: ico.ICONDIR{Reserved: 0, Type: 1, Count: uint16(len(frames))}}
	for i, f := range frames {
		id := firstIconID + uint16(i)
		data := make([]byte, f.BytesInRes)
		if _, err := iconFile.ReadAt(data, int64(f.ImageOffset)); err != nil {
			return fmt.Errorf("%s: frame %d: %w", iconPath, i, err)
		}
		rsrc.AddResource(rtIcon, id, bytes.NewReader(data))
		group.Entries = append(group.Entries, grpIconDirEntry{IconDirEntryCommon: f.IconDirEntryCommon, ID: id})
	}
	rsrc.AddResource(rtGroupIcon, groupIconID, group)

	rsrc.AddResource(rtVersion, versionID, goversioninfo.SizedReader{Buffer: bytes.NewBuffer(vi.Buffer.Bytes())})

	man, err := binutil.SizedOpen(manifestPath)
	if err != nil {
		return err
	}
	defer man.Close()
	rsrc.AddResource(rtManifest, manifestID, man)

	// Freeze resolves every internal offset. Nothing may be added afterwards.
	rsrc.Freeze()

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	bw := binutil.Writer{W: f}
	binutil.Walk(rsrc, func(v reflect.Value, path string) error {
		if binutil.Plain(v.Kind()) {
			bw.WriteLE(v.Interface())
			return nil
		}
		if sr, ok := v.Interface().(binutil.SizedReader); ok {
			bw.WriteFromSized(sr)
			return binutil.WALK_SKIP
		}
		return nil
	})
	if bw.Err != nil {
		f.Close()
		return fmt.Errorf("writing %s: %w", outPath, bw.Err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	st, err := os.Stat(outPath)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes, %s)\n", outPath, st.Size(), arch)
	fmt.Printf("  RT_ICON       %d frames, ids %d..%d\n", len(frames), firstIconID, firstIconID+uint16(len(frames))-1)
	fmt.Printf("  RT_GROUP_ICON id %d\n", groupIconID)
	fmt.Printf("  RT_VERSION    id %d, %s\n", versionID, vi.FixedFileInfo.FileVersion.GetVersionString())
	fmt.Printf("  RT_MANIFEST   id %d, %s (%d bytes)\n", manifestID, manifestPath, man.Size())
	return nil
}

func loadVersionInfo(path, version string) (*goversioninfo.VersionInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	vi := &goversioninfo.VersionInfo{}
	if err := vi.ParseJSON(raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// The string fields keep the version verbatim - that is what the
	// Properties dialog shows, and it is the same text -ldflags injects into
	// internal/build.Version. The fixed fields are four uint16s and cannot
	// hold a suffix, so they get the numeric prefix.
	vi.StringFileInfo.FileVersion = version
	vi.StringFileInfo.ProductVersion = version

	fv, err := parseFileVersion(version)
	if err != nil {
		return nil, err
	}
	vi.FixedFileInfo.FileVersion = fv
	vi.FixedFileInfo.ProductVersion = fv
	return vi, nil
}

// verRe matches a leading dotted numeric version. Two to four components are
// accepted so that a bare `1.0` tag does not fail the build; missing trailing
// components are zero, which is what every other Windows toolchain does.
var verRe = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

func parseFileVersion(s string) (goversioninfo.FileVersion, error) {
	m := verRe.FindStringSubmatch(s)
	if m == nil {
		return goversioninfo.FileVersion{}, fmt.Errorf("version %q does not start with a dotted number such as 1.0.15", s)
	}
	var n [4]int
	for i := 0; i < 4; i++ {
		if m[i+1] == "" {
			continue
		}
		v, err := strconv.Atoi(m[i+1])
		if err != nil || v > 65535 {
			return goversioninfo.FileVersion{}, fmt.Errorf("version %q: component %q must be 0..65535", s, m[i+1])
		}
		n[i] = v
	}
	if m[0] != s {
		fmt.Fprintf(os.Stderr, "note: version %q is not purely numeric; VS_FIXEDFILEINFO gets %d.%d.%d.%d, the Properties dialog shows the full string\n",
			s, n[0], n[1], n[2], n[3])
	}
	return goversioninfo.FileVersion{Major: n[0], Minor: n[1], Patch: n[2], Build: n[3]}, nil
}

// grpIconDir is the RT_GROUP_ICON payload: an ICONDIR whose entries point at
// RT_ICON resource IDs instead of at file offsets. Shape per
// https://devblogs.microsoft.com/oldnewthing/20120720-00/?p=7083 - the entry is
// the 14-byte ICONDIRENTRY with the 4-byte file offset replaced by a 2-byte
// resource ID, so the struct is 14 bytes on disk, not 16. binutil.Walk writes
// the fields in declaration order with no Go padding, which is why this is
// declared as plain fields rather than serialised with encoding/binary.
type grpIconDir struct {
	ico.ICONDIR
	Entries []grpIconDirEntry
}

type grpIconDirEntry struct {
	ico.IconDirEntryCommon
	ID uint16
}

// Size reports the on-disk length; coff.AddResource stores it in the
// IMAGE_RESOURCE_DATA_ENTRY, so it has to match what Walk actually writes.
func (g grpIconDir) Size() int64 {
	return int64(binary.Size(ico.ICONDIR{}) + len(g.Entries)*binary.Size(grpIconDirEntry{}))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "winres:", err)
	os.Exit(1)
}
