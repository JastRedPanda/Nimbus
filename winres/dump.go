package main

// Verification side of the tool: parse a .syso back out and print what is
// actually in it. This machine has no Windows and no mingw windres, so this is
// the only way to check the object before it ships. It reads the COFF with
// debug/pe from the standard library and walks the IMAGE_RESOURCE_DIRECTORY
// tree by hand.
//
// It also asserts the invariants that would otherwise only fail on a user's
// desktop: manifest under RT_MANIFEST id 1, icon group under id 1, version
// block with the right signature, and ID directories in ascending order (the
// PE loader binary-searches them, so an out-of-order directory is a resource
// that silently cannot be found).

import (
	"debug/pe"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf16"
)

var resourceTypeNames = map[uint32]string{
	1: "RT_CURSOR", 2: "RT_BITMAP", 3: "RT_ICON", 4: "RT_MENU", 5: "RT_DIALOG",
	6: "RT_STRING", 7: "RT_FONTDIR", 8: "RT_FONT", 9: "RT_ACCELERATOR",
	10: "RT_RCDATA", 11: "RT_MESSAGETABLE", 12: "RT_GROUP_CURSOR",
	14: "RT_GROUP_ICON", 16: "RT_VERSION", 17: "RT_DLGINCLUDE",
	19: "RT_PLUGPLAY", 20: "RT_VXD", 21: "RT_ANICURSOR", 22: "RT_ANIICON",
	23: "RT_HTML", 24: "RT_MANIFEST",
}

type resDirEntry struct {
	nameOrID uint32
	offset   uint32
}

const subdirFlag = uint32(1) << 31

func dumpSyso(path string, w io.Writer) error {
	f, err := pe.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	machine := map[uint16]string{
		pe.IMAGE_FILE_MACHINE_I386:  "i386",
		pe.IMAGE_FILE_MACHINE_AMD64: "amd64",
		pe.IMAGE_FILE_MACHINE_ARMNT: "arm",
		pe.IMAGE_FILE_MACHINE_ARM64: "arm64",
	}[f.Machine]
	if machine == "" {
		machine = "unknown"
	}
	fmt.Fprintf(w, "%s\n", path)
	fmt.Fprintf(w, "  machine 0x%04x (%s), %d section(s), %d symbol(s)\n",
		f.Machine, machine, f.NumberOfSections, f.NumberOfSymbols)

	b, root, payloadAt, err := resourceBlob(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if sec := f.Section(".rsrc"); sec != nil {
		// One relocation per resource in an object file, and the Go linker
		// refuses more than one .rsrc section, so this is worth showing.
		fmt.Fprintf(w, "  .rsrc %d bytes, %d relocation(s)\n", sec.Size, sec.NumberOfRelocations)
	}
	fmt.Fprintln(w)

	types, err := readResDir(b, root)
	if err != nil {
		return err
	}
	problems := []string{}
	seen := map[uint32]map[uint16]bool{}

	for _, t := range types {
		name := resourceTypeNames[t.nameOrID]
		if name == "" {
			name = fmt.Sprintf("type %d", t.nameOrID)
		}
		fmt.Fprintf(w, "%s\n", name)
		if t.offset&subdirFlag == 0 {
			problems = append(problems, fmt.Sprintf("%s: leaf where a subdirectory was expected", name))
			continue
		}
		ids, err := readResDir(b, root+(t.offset&^subdirFlag))
		if err != nil {
			return err
		}
		if !sort.SliceIsSorted(ids, func(i, j int) bool { return ids[i].nameOrID < ids[j].nameOrID }) {
			problems = append(problems, fmt.Sprintf("%s: ID directory is not sorted ascending; the loader binary-searches it", name))
		}
		seen[t.nameOrID] = map[uint16]bool{}
		for _, id := range ids {
			seen[t.nameOrID][uint16(id.nameOrID)] = true
			langs, err := readResDir(b, root+(id.offset&^subdirFlag))
			if err != nil {
				return err
			}
			for _, lg := range langs {
				off := root + (lg.offset &^ subdirFlag)
				if int(off)+16 > len(b) {
					return fmt.Errorf("data entry offset %d out of range", off)
				}
				dataOff := binary.LittleEndian.Uint32(b[off:])
				size := binary.LittleEndian.Uint32(b[off+4:])
				fmt.Fprintf(w, "  id %-5d lang 0x%04x  %d bytes\n", id.nameOrID, lg.nameOrID, size)
				start, ok := payloadAt(dataOff)
				if !ok || start+int(size) > len(b) {
					problems = append(problems, fmt.Sprintf("%s id %d: payload runs past the end of the resource section", name, id.nameOrID))
					continue
				}
				data := b[start : start+int(size)]
				switch t.nameOrID {
				case rtManifest:
					problems = append(problems, describeManifest(w, data)...)
				case rtVersion:
					problems = append(problems, describeVersion(w, data)...)
				case rtGroupIcon:
					problems = append(problems, describeGroupIcon(w, data, seen[rtIcon])...)
				}
			}
		}
	}

	fmt.Fprintln(w)
	if !seen[rtManifest][manifestID] {
		problems = append(problems, fmt.Sprintf("no RT_MANIFEST under id %d (CREATEPROCESS_MANIFEST_RESOURCE_ID); Windows will not read it", manifestID))
	}
	if !seen[rtGroupIcon][groupIconID] {
		problems = append(problems, fmt.Sprintf("no RT_GROUP_ICON under id %d; LoadIcon(MAKEINTRESOURCE(%d)) in internal/ui returns NULL", groupIconID, groupIconID))
	}
	if !seen[rtVersion][versionID] {
		problems = append(problems, fmt.Sprintf("no RT_VERSION under id %d; the Properties dialog will show no version tab", versionID))
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(w, "PROBLEM: %s\n", p)
		}
		return fmt.Errorf("%d problem(s) found", len(problems))
	}
	fmt.Fprintln(w, "OK: manifest, icon group and version block are all present under the IDs the code and the OS look for")
	return nil
}

// resourceBlob locates the resource directory in either an unlinked COFF
// object (a .syso, which is what this tool writes) or a linked PE image (the
// .exe the Go linker produces from it). The two differ in how the offsets are
// expressed: in an object everything is relative to the start of .rsrc, in an
// image the IMAGE_RESOURCE_DATA_ENTRY pointers are RVAs. Returning the section
// bytes, the offset of the root directory within them, and a translator for
// payload pointers is enough to walk both with the same code.
func resourceBlob(f *pe.File) (b []byte, root uint32, payloadAt func(uint32) (int, bool), err error) {
	var dir pe.DataDirectory
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		dir = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE]
	case *pe.OptionalHeader32:
		dir = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE]
	default:
		// No optional header: an object file. rsrc emits offsets relative to
		// the start of the section, so no translation is needed.
		sec := f.Section(".rsrc")
		if sec == nil {
			return nil, 0, nil, fmt.Errorf("no .rsrc section - the Go linker would find nothing to embed here")
		}
		b, err = sec.Data()
		if err != nil {
			return nil, 0, nil, err
		}
		return b, 0, func(o uint32) (int, bool) { return int(o), int(o) <= len(b) }, nil
	}

	if dir.VirtualAddress == 0 || dir.Size == 0 {
		return nil, 0, nil, fmt.Errorf("image has no resource data directory - the .syso was not linked in")
	}
	for _, sec := range f.Sections {
		if dir.VirtualAddress < sec.VirtualAddress || dir.VirtualAddress >= sec.VirtualAddress+sec.VirtualSize {
			continue
		}
		b, err = sec.Data()
		if err != nil {
			return nil, 0, nil, err
		}
		va := sec.VirtualAddress
		return b, dir.VirtualAddress - va, func(o uint32) (int, bool) {
			if o < va {
				return 0, false
			}
			return int(o - va), int(o-va) <= len(b)
		}, nil
	}
	return nil, 0, nil, fmt.Errorf("resource directory RVA 0x%x is in no section", dir.VirtualAddress)
}

func readResDir(b []byte, off uint32) ([]resDirEntry, error) {
	if int(off)+16 > len(b) {
		return nil, fmt.Errorf("resource directory at %d out of range", off)
	}
	named := binary.LittleEndian.Uint16(b[off+12:])
	ids := binary.LittleEndian.Uint16(b[off+14:])
	p := off + 16
	out := make([]resDirEntry, 0, int(named)+int(ids))
	for i := 0; i < int(named)+int(ids); i++ {
		if int(p)+8 > len(b) {
			return nil, fmt.Errorf("resource directory entry at %d out of range", p)
		}
		out = append(out, resDirEntry{
			nameOrID: binary.LittleEndian.Uint32(b[p:]),
			offset:   binary.LittleEndian.Uint32(b[p+4:]),
		})
		p += 8
	}
	// Only ID entries are expected here; named entries would come first and
	// this tool never emits them.
	if named != 0 {
		return nil, fmt.Errorf("unexpected named resource entries (%d)", named)
	}
	return out, nil
}

func describeManifest(w io.Writer, data []byte) []string {
	var problems []string
	var doc struct {
		XMLName      xml.Name
		Dependencies []struct {
			Ident struct {
				Type           string `xml:"type,attr"`
				Name           string `xml:"name,attr"`
				Version        string `xml:"version,attr"`
				PublicKeyToken string `xml:"publicKeyToken,attr"`
			} `xml:"dependentAssembly>assemblyIdentity"`
		} `xml:"dependency"`
		Compatibility struct {
			SupportedOS []struct {
				ID string `xml:"Id,attr"`
			} `xml:"application>supportedOS"`
		} `xml:"compatibility"`
		Application struct {
			WindowsSettings struct {
				DpiAware     string `xml:"dpiAware"`
				DpiAwareness string `xml:"dpiAwareness"`
				LongPath     string `xml:"longPathAware"`
			} `xml:"windowsSettings"`
		} `xml:"application"`
		Trust struct {
			Level struct {
				Level string `xml:"level,attr"`
			} `xml:"security>requestedPrivileges>requestedExecutionLevel"`
		} `xml:"trustInfo"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		problems = append(problems, fmt.Sprintf("manifest is not well-formed XML: %v", err))
		fmt.Fprintf(w, "    !! %v\n", err)
		return problems
	}
	fmt.Fprintf(w, "    root element   <%s> in %s\n", doc.XMLName.Local, doc.XMLName.Space)
	if doc.XMLName.Local != "assembly" || doc.XMLName.Space != "urn:schemas-microsoft-com:asm.v1" {
		problems = append(problems, "manifest root is not {urn:schemas-microsoft-com:asm.v1}assembly")
	}

	comctl := false
	for _, d := range doc.Dependencies {
		fmt.Fprintf(w, "    dependency     %s %s (token %s)\n", d.Ident.Name, d.Ident.Version, d.Ident.PublicKeyToken)
		if d.Ident.Name == "Microsoft.Windows.Common-Controls" &&
			d.Ident.Version == "6.0.0.0" && d.Ident.PublicKeyToken == "6595b64144ccf1df" {
			comctl = true
		}
	}
	if !comctl {
		problems = append(problems, "no Microsoft.Windows.Common-Controls 6.0.0.0 dependency; the UI will render with comctl32 v5 (Windows Classic)")
	}

	ws := doc.Application.WindowsSettings
	fmt.Fprintf(w, "    dpiAware       %q\n", ws.DpiAware)
	fmt.Fprintf(w, "    dpiAwareness   %q\n", ws.DpiAwareness)
	fmt.Fprintf(w, "    longPathAware  %q\n", ws.LongPath)
	fmt.Fprintf(w, "    execution      %q\n", doc.Trust.Level.Level)
	if ws.DpiAware == "" && ws.DpiAwareness == "" {
		problems = append(problems, "no DPI awareness declared; the process stays DPI-unaware and gets bitmap-stretched")
	}

	ids := make([]string, 0, len(doc.Compatibility.SupportedOS))
	for _, o := range doc.Compatibility.SupportedOS {
		ids = append(ids, o.ID)
	}
	fmt.Fprintf(w, "    supportedOS    %d entr(y/ies)\n", len(ids))
	if !containsFold(ids, "{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}") {
		problems = append(problems, "supportedOS does not list the Windows 10/11 GUID {8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}")
	}
	return problems
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}

func describeVersion(w io.Writer, d []byte) []string {
	var problems []string
	if len(d) < 6 {
		return []string{"RT_VERSION payload is too short to be a VS_VERSIONINFO"}
	}
	length := binary.LittleEndian.Uint16(d[0:])
	key := utf16z(d[6:])
	if key != "VS_VERSION_INFO" {
		problems = append(problems, fmt.Sprintf("VS_VERSIONINFO key is %q, want %q", key, "VS_VERSION_INFO"))
	}
	if int(length) != len(d) {
		problems = append(problems, fmt.Sprintf("VS_VERSIONINFO wLength is %d but the resource is %d bytes", length, len(d)))
	}

	// VS_FIXEDFILEINFO begins at the next 32-bit boundary after the key.
	off := (6 + (len(key)+1)*2 + 3) &^ 3
	if off+52 > len(d) {
		return append(problems, "no room for VS_FIXEDFILEINFO")
	}
	sig := binary.LittleEndian.Uint32(d[off:])
	if sig != 0xFEEF04BD {
		problems = append(problems, fmt.Sprintf("VS_FIXEDFILEINFO signature is 0x%08x, want 0xFEEF04BD", sig))
	}
	fvMS := binary.LittleEndian.Uint32(d[off+8:])
	fvLS := binary.LittleEndian.Uint32(d[off+12:])
	pvMS := binary.LittleEndian.Uint32(d[off+16:])
	pvLS := binary.LittleEndian.Uint32(d[off+20:])
	fmt.Fprintf(w, "    key            %q, wLength %d\n", key, length)
	fmt.Fprintf(w, "    FileVersion    %d.%d.%d.%d\n", fvMS>>16, fvMS&0xffff, fvLS>>16, fvLS&0xffff)
	fmt.Fprintf(w, "    ProductVersion %d.%d.%d.%d\n", pvMS>>16, pvMS&0xffff, pvLS>>16, pvLS&0xffff)
	fmt.Fprintf(w, "    FileOS 0x%08x FileType 0x%08x\n",
		binary.LittleEndian.Uint32(d[off+32:]), binary.LittleEndian.Uint32(d[off+36:]))

	// Walk the child blocks properly rather than scanning for legible text:
	// this is the check that the Properties dialog will actually find the
	// strings, and a truncated or misaligned block shows up as a parse error
	// instead of as plausible-looking output.
	root, err := parseVerBlock(d, 0)
	if err != nil {
		return append(problems, fmt.Sprintf("VS_VERSIONINFO: %v", err))
	}
	sawStrings, sawTranslation := false, false
	for _, child := range root.children(d) {
		switch child.key {
		case "StringFileInfo":
			for _, table := range child.children(d) {
				fmt.Fprintf(w, "    string table   %s\n", table.key)
				for _, s := range table.children(d) {
					fmt.Fprintf(w, "      %-17s %s\n", s.key, s.text(d))
					sawStrings = true
				}
			}
		case "VarFileInfo":
			for _, v := range child.children(d) {
				raw := d[v.valueOff : v.valueOff+v.valueBytes]
				if v.key == "Translation" && len(raw) >= 4 {
					fmt.Fprintf(w, "    translation    lang 0x%04x charset 0x%04x\n",
						binary.LittleEndian.Uint16(raw), binary.LittleEndian.Uint16(raw[2:]))
					sawTranslation = true
				}
			}
		default:
			fmt.Fprintf(w, "    unknown block  %q\n", child.key)
		}
	}
	if !sawStrings {
		problems = append(problems, "VS_VERSIONINFO carries no StringFileInfo entries; the Properties dialog would show an empty Details tab")
	}
	if !sawTranslation {
		problems = append(problems, "VS_VERSIONINFO carries no VarFileInfo/Translation; Explorer may not find the string table")
	}
	return problems
}

// verBlock is one node of the VS_VERSIONINFO tree. Every node has the same
// shape: a 6-byte header, a NUL-terminated UTF-16 key, then an optional value
// and optional children, each starting on a 32-bit boundary measured from the
// start of the resource.
type verBlock struct {
	off, end   int
	key        string
	wType      uint16
	valueOff   int
	valueBytes int
	childOff   int
}

func align4(n int) int { return (n + 3) &^ 3 }

func parseVerBlock(d []byte, off int) (verBlock, error) {
	if off+6 > len(d) {
		return verBlock{}, fmt.Errorf("block at %d truncated", off)
	}
	b := verBlock{off: off}
	length := int(binary.LittleEndian.Uint16(d[off:]))
	valueLen := int(binary.LittleEndian.Uint16(d[off+2:]))
	b.wType = binary.LittleEndian.Uint16(d[off+4:])
	if length < 6 || off+length > len(d) {
		return verBlock{}, fmt.Errorf("block at %d claims %d bytes, resource has %d", off, length, len(d))
	}
	b.end = off + length
	b.key = utf16z(d[off+6:])
	b.valueOff = align4(off + 6 + (len(b.key)+1)*2)
	// wValueLength counts WCHARs for text values (wType 1) and bytes for
	// binary ones (wType 0). Getting this backwards silently shifts every
	// following sibling.
	b.valueBytes = valueLen
	if b.wType == 1 {
		b.valueBytes = valueLen * 2
	}
	if b.valueOff+b.valueBytes > len(d) {
		return verBlock{}, fmt.Errorf("block %q value runs past the resource", b.key)
	}
	b.childOff = align4(b.valueOff + b.valueBytes)
	return b, nil
}

func (b verBlock) children(d []byte) []verBlock {
	var out []verBlock
	for off := b.childOff; off+6 <= b.end; {
		c, err := parseVerBlock(d, off)
		if err != nil || c.end <= off {
			return out
		}
		out = append(out, c)
		off = align4(c.end)
	}
	return out
}

func (b verBlock) text(d []byte) string {
	if b.valueBytes == 0 {
		return `""`
	}
	return fmt.Sprintf("%q", utf16z(d[b.valueOff:b.valueOff+b.valueBytes]))
}

func describeGroupIcon(w io.Writer, d []byte, iconIDs map[uint16]bool) []string {
	var problems []string
	if len(d) < 6 {
		return []string{"RT_GROUP_ICON payload is too short"}
	}
	reserved := binary.LittleEndian.Uint16(d[0:])
	typ := binary.LittleEndian.Uint16(d[2:])
	count := binary.LittleEndian.Uint16(d[4:])
	if reserved != 0 || typ != 1 {
		problems = append(problems, fmt.Sprintf("GRPICONDIR header is reserved=%d type=%d, want 0 and 1", reserved, typ))
	}
	if want := 6 + int(count)*14; want != len(d) {
		problems = append(problems, fmt.Sprintf("GRPICONDIR is %d bytes, want %d for %d entries", len(d), want, count))
	}
	for i := 0; i < int(count) && 6+i*14+14 <= len(d); i++ {
		p := 6 + i*14
		wpx, hpx := int(d[p]), int(d[p+1])
		if wpx == 0 {
			wpx = 256
		}
		if hpx == 0 {
			hpx = 256
		}
		bits := binary.LittleEndian.Uint16(d[p+6:])
		size := binary.LittleEndian.Uint32(d[p+8:])
		id := binary.LittleEndian.Uint16(d[p+12:])
		fmt.Fprintf(w, "    %3dx%-3d %2dbpp %7d bytes -> RT_ICON id %d\n", wpx, hpx, bits, size, id)
		if iconIDs != nil && !iconIDs[id] {
			problems = append(problems, fmt.Sprintf("GRPICONDIR entry %d points at RT_ICON id %d, which does not exist", i, id))
		}
	}
	return problems
}

func utf16z(b []byte) string {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i:])
		if v == 0 {
			break
		}
		u = append(u, v)
	}
	return string(utf16.Decode(u))
}
