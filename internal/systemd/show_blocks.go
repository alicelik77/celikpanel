package systemd

import "strings"

// ParseShowBlocks splits the answer of one `systemctl show -p Id,… u1 u2 …`
// into one property map per unit, in the order the units were asked
// (alpha.125: one process for every unit of a scan instead of one each).
// systemd separates units by one blank line and prints an empty property only
// with --all, so a block can never be empty while Id is asked: every unit,
// even one that does not exist, has an Id. ok=false unless there is exactly
// one block per unit and each block has one non-empty Id; the caller then
// asks unit by unit instead of guessing whose block is whose.
//
// ParseShowBlocks, toplu `systemctl show` yanıtını sorulan sırayla birim
// başına özellik haritalarına böler. Her birim için tam bir blok ve her blokta
// tek, boş olmayan Id yoksa ok=false döner; çağıran birim birim sorar.
func ParseShowBlocks(output string, units int) ([]map[string][]string, bool) {
	if units <= 0 {
		return nil, false
	}
	text := strings.TrimRight(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	if text == "" {
		return nil, false
	}
	chunks := strings.Split(text, "\n\n")
	if len(chunks) != units {
		return nil, false
	}
	blocks := make([]map[string][]string, 0, units)
	for _, chunk := range chunks {
		block := map[string][]string{}
		for _, line := range strings.Split(chunk, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok || key == "" {
				return nil, false
			}
			block[key] = append(block[key], value)
		}
		if ids := block["Id"]; len(ids) != 1 || strings.TrimSpace(ids[0]) == "" {
			return nil, false
		}
		blocks = append(blocks, block)
	}
	return blocks, true
}
