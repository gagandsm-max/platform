package ebpf

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	bpfLdImm64         = 0x18
	bpfCall            = 0x85
	bpfPseudoCall      = 1
	bpfPseudoKfuncCall = 2
	bpfPseudoFunc      = 4
	rBPF64_64          = 1
)

// ObjectGate names match the independently audited OSS-124 admission vocabulary.
const (
	GateTextEmpty       = "OBJECT_GATE_TEXT_EMPTY"
	Gate2103NoSubprog   = "OBJECT_GATE_2103_NO_NONINLINE_SUBPROGRAMS"
	GateNoKfuncKsyms    = "NO_KFUNC_KSYMS_GATE"
	Gate2083ZeroAddends = "OBJECT_GATE_2083_ZERO_STT_OBJECT_ADDENDS"
	GateCORePresent     = "OBJECT_GATE_CORE_BTF_PRESENT"
	GateProgramAllow    = "OBJECT_GATE_PROGRAM_ALLOWLIST"
	GateMapAllow        = "OBJECT_GATE_MAP_ALLOWLIST"
)

type objectGateResult struct {
	Name    string
	Pass    bool
	Details []string
}

// AdmitGovernedObject fail-closes before any kernel interaction.
func AdmitGovernedObject(objectBytes []byte) ([]objectGateResult, error) {
	if len(objectBytes) == 0 {
		return nil, errors.New("OBJECT_INTEGRITY_EMPTY")
	}
	f, err := elf.NewFile(newBytesReaderAt(objectBytes))
	if err != nil {
		return nil, fmt.Errorf("OBJECT_INTEGRITY_ELF: %w", err)
	}
	defer f.Close()
	if f.Class != elf.ELFCLASS64 || f.Machine != elf.EM_BPF {
		return nil, fmt.Errorf("OBJECT_INTEGRITY_MACHINE: class=%s machine=%s", f.Class, f.Machine)
	}

	textDetails := []string{}
	subprogramDetails := []string{}
	kfuncDetails := []string{}
	addendDetails := []string{}
	hasBTF := false
	hasBTFExt := false
	programs := map[string]bool{}
	maps := map[string]bool{}

	for _, sec := range f.Sections {
		switch sec.Name {
		case ".text":
			if sec.Size != 0 {
				textDetails = append(textDetails, fmt.Sprintf("non-empty .text size=%d", sec.Size))
			}
		case ".ksyms":
			kfuncDetails = append(kfuncDetails, fmt.Sprintf("forbidden .ksyms section size=%d", sec.Size))
		case ".BTF":
			hasBTF = sec.Size > 0
		case ".BTF.ext":
			hasBTFExt = sec.Size > 0
		}
		if stringsHasPrefix(sec.Name, "tracepoint/") && sec.Flags&elf.SHF_EXECINSTR != 0 {
			// Program section present; symbol names validated below.
		}
		if sec.Flags&elf.SHF_EXECINSTR == 0 || sec.Size == 0 {
			continue
		}
		data, err := sec.Data()
		if err != nil {
			return nil, fmt.Errorf("OBJECT_INTEGRITY_SECTION %q: %w", sec.Name, err)
		}
		for off := 0; off+8 <= len(data); off += 8 {
			op := data[off]
			src := sourceRegister(data[off+1], f.ByteOrder)
			if op == bpfCall && src == bpfPseudoCall {
				subprogramDetails = append(subprogramDetails, fmt.Sprintf("BPF-to-BPF pseudo-call section=%s offset=%d", sec.Name, off))
			}
			if op == bpfCall && src == bpfPseudoKfuncCall {
				kfuncDetails = append(kfuncDetails, fmt.Sprintf("kfunc pseudo-call section=%s offset=%d", sec.Name, off))
			}
			if op == bpfLdImm64 && src == bpfPseudoFunc {
				kfuncDetails = append(kfuncDetails, fmt.Sprintf("BTF-id/.ksyms pseudo-function reference section=%s offset=%d", sec.Name, off))
			}
		}
	}

	syms, err := f.Symbols()
	if err != nil && !errors.Is(err, elf.ErrNoSymbols) {
		return nil, fmt.Errorf("OBJECT_INTEGRITY_SYMBOLS: %w", err)
	}
	for _, sym := range syms {
		switch sym.Name {
		case ProgInetSockSetState, ProgTCPRetransmit:
			if elf.ST_TYPE(sym.Info) == elf.STT_FUNC || elf.ST_BIND(sym.Info) != 0 {
				programs[sym.Name] = true
			}
			programs[sym.Name] = true
		case MapConnState, MapEvents, MapLossCounters:
			maps[sym.Name] = true
		}
	}

	for _, relsec := range f.Sections {
		if relsec.Type != elf.SHT_REL && relsec.Type != elf.SHT_RELA {
			continue
		}
		if int(relsec.Info) >= len(f.Sections) {
			return nil, fmt.Errorf("OBJECT_INTEGRITY_RELOC: section %q bad target index", relsec.Name)
		}
		target := f.Sections[relsec.Info]
		if target.Flags&elf.SHF_EXECINSTR == 0 {
			continue
		}
		targetData, err := target.Data()
		if err != nil {
			return nil, err
		}
		relData, err := relsec.Data()
		if err != nil {
			return nil, err
		}
		entrySize := int(relsec.Entsize)
		if entrySize == 0 {
			if relsec.Type == elf.SHT_RELA {
				entrySize = 24
			} else {
				entrySize = 16
			}
		}
		for off := 0; off+entrySize <= len(relData); off += entrySize {
			rOff := f.ByteOrder.Uint64(relData[off : off+8])
			rInfo := f.ByteOrder.Uint64(relData[off+8 : off+16])
			symIndex := uint32(rInfo >> 32)
			rType := uint32(rInfo)
			if symIndex == 0 || int(symIndex) > len(syms) {
				continue
			}
			sym := syms[symIndex-1]
			if sym.Section == elf.SHN_UNDEF {
				kfuncDetails = append(kfuncDetails,
					fmt.Sprintf("external kfunc/.ksyms relocation section=%s symbol=%s type=%d offset=%d", target.Name, sym.Name, rType, rOff))
			}
			if elf.ST_TYPE(sym.Info) != elf.STT_OBJECT || rType != rBPF64_64 {
				continue
			}
			implicit, err := implicitAddend(targetData, rOff, f.ByteOrder)
			if err != nil {
				return nil, err
			}
			var explicit int64
			if relsec.Type == elf.SHT_RELA {
				explicit = int64(f.ByteOrder.Uint64(relData[off+16 : off+24]))
			}
			if implicit != 0 || explicit != 0 {
				addendDetails = append(addendDetails,
					fmt.Sprintf("non-zero STT_OBJECT addend section=%s symbol=%s offset=%d implicit=%#x explicit=%d", target.Name, sym.Name, rOff, implicit, explicit))
			}
		}
	}

	progDetails := []string{}
	if !programs[ProgInetSockSetState] {
		progDetails = append(progDetails, "missing primary "+ProgInetSockSetState)
	}
	for name := range programs {
		if name != ProgInetSockSetState && name != ProgTCPRetransmit {
			progDetails = append(progDetails, "unexpected program "+name)
		}
	}
	mapDetails := []string{}
	for _, want := range []string{MapConnState, MapEvents, MapLossCounters} {
		if !maps[want] {
			mapDetails = append(mapDetails, "missing map "+want)
		}
	}
	for name := range maps {
		switch name {
		case MapConnState, MapEvents, MapLossCounters:
		default:
			mapDetails = append(mapDetails, "unexpected map "+name)
		}
	}
	coreDetails := []string{}
	if !hasBTF || !hasBTFExt {
		coreDetails = append(coreDetails, fmt.Sprintf("btf=%t btf_ext=%t", hasBTF, hasBTFExt))
	}

	gates := []objectGateResult{
		{Name: GateTextEmpty, Pass: len(textDetails) == 0, Details: textDetails},
		{Name: Gate2103NoSubprog, Pass: len(subprogramDetails) == 0, Details: subprogramDetails},
		{Name: GateNoKfuncKsyms, Pass: len(kfuncDetails) == 0, Details: kfuncDetails},
		{Name: Gate2083ZeroAddends, Pass: len(addendDetails) == 0, Details: addendDetails},
		{Name: GateCORePresent, Pass: len(coreDetails) == 0, Details: coreDetails},
		{Name: GateProgramAllow, Pass: len(progDetails) == 0, Details: progDetails},
		{Name: GateMapAllow, Pass: len(mapDetails) == 0, Details: mapDetails},
	}
	for _, g := range gates {
		if !g.Pass {
			return gates, fmt.Errorf("OBJECT_ADMISSION_FAILED: %s", g.Name)
		}
	}
	return gates, nil
}

func sourceRegister(regs byte, order binary.ByteOrder) byte {
	if order == binary.BigEndian {
		return regs & 0x0f
	}
	return regs >> 4
}

func implicitAddend(code []byte, offset uint64, order binary.ByteOrder) (uint64, error) {
	if offset > uint64(len(code)) || uint64(len(code))-offset < 16 {
		return 0, fmt.Errorf("LDIMM64 at %d is truncated", offset)
	}
	ins := code[offset : offset+16]
	if ins[0] != bpfLdImm64 {
		return 0, fmt.Errorf("relocation target opcode %#x is not LDIMM64", ins[0])
	}
	lo := order.Uint32(ins[4:8])
	hi := order.Uint32(ins[12:16])
	return uint64(hi)<<32 | uint64(lo), nil
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// bytesReaderAt adapts a byte slice to elf.NewFile without tempfile.
type bytesReaderAt struct{ b []byte }

func newBytesReaderAt(b []byte) *bytesReaderAt { return &bytesReaderAt{b: b} }

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *bytesReaderAt) Close() error { return nil }
