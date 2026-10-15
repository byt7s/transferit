package download

import (
	"fmt"
	"sort"
	"strings"

	"transferit/internal/api"
	"transferit/internal/crypto"
	"transferit/internal/format"
)

func (d *Downloader) Info(input, password string) error {
	xh, err := d.extractHandle(input)
	if err != nil {
		return err
	}
	info, err := d.getTransferInfo(xh)
	if err != nil {
		return err
	}
	nodes, err := d.fetchFileList(xh, password)
	switch {
	case err == nil:
		fmt.Print(renderInfo(xh, info, d.decodeNodes(nodes)))
		return nil
	case err == ErrPasswordRequired && password == "":
		fmt.Print(renderInfo(xh, info, nil))
		return nil
	case err == ErrPasswordRequired:
		return fmt.Errorf("wrong password")
	default:
		return err
	}
}

func renderInfo(xh string, info *api.TransferInfo, nodes []api.FileNode) string {
	var b strings.Builder
	writeField(&b, "link", api.Web+"/t/"+xh)
	if title := decodedText(info.T); title != "" {
		writeField(&b, "title", title)
	}
	if sender := strings.TrimSpace(info.SE); sender != "" {
		writeField(&b, "sender", sender)
	}
	if message := decodedText(info.M); message != "" {
		writeMessage(&b, message)
	}
	if passwordRequired(info.PW) {
		writeField(&b, "password", "required")
	} else {
		writeField(&b, "password", "none")
	}
	files, folders := countNodes(nodes)
	if nodes == nil {
		files, folders = countFromInfo(info)
	}
	writeField(&b, "size", sizeLine(transferBytes(info), files, folders))
	b.WriteByte('\n')
	if nodes == nil {
		if passwordRequired(info.PW) {
			b.WriteString("  pass -p to list the files\n")
		}
		return b.String()
	}
	b.WriteString(renderTree(nodes))
	return b.String()
}

func writeField(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "  %-10s%s\n", label, value)
}

func writeMessage(b *strings.Builder, message string) {
	if !strings.Contains(message, "\n") {
		writeField(b, "message", message)
		return
	}
	writeField(b, "message", "")
	for _, line := range strings.Split(message, "\n") {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

func decodedText(raw string) string {
	if raw == "" {
		return ""
	}
	buf, err := crypto.Base64URLDecode(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(buf))
}

func passwordRequired(raw []byte) bool {
	switch strings.TrimSpace(string(raw)) {
	case "", "null", "0", "false", `""`:
		return false
	default:
		return true
	}
}

func countFromInfo(info *api.TransferInfo) (files, folders int) {
	if info == nil || len(info.Size) < 2 {
		return 0, 0
	}
	files = int(info.Size[1])
	if len(info.Size) > 2 && info.Size[2] > 0 {
		folders = int(info.Size[2] - 1)
	}
	if folders < 0 {
		folders = 0
	}
	return files, folders
}

func countNodes(nodes []api.FileNode) (files, folders int) {
	roots := rootSet(nodes)
	for _, n := range nodes {
		if roots[n.H] && n.Type == "folder" {
			continue
		}
		if n.Type == "folder" {
			folders++
			continue
		}
		files++
	}
	return files, folders
}

func sizeLine(total int64, files, folders int) string {
	parts := []string{format.Size(total), countWord(files, "file", "files")}
	if folders > 0 {
		parts = append(parts, countWord(folders, "folder", "folders"))
	}
	return strings.Join(parts, "  ·  ")
}

func countWord(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func renderTree(nodes []api.FileNode) string {
	if len(nodes) == 0 {
		return "  no files\n"
	}
	byParent := map[string][]api.FileNode{}
	for _, n := range nodes {
		byParent[n.P] = append(byParent[n.P], n)
	}
	for parent, kids := range byParent {
		byParent[parent] = sortNodes(kids)
	}
	var roots []api.FileNode
	known := map[string]bool{}
	for _, n := range nodes {
		known[n.H] = true
	}
	for _, n := range nodes {
		if n.P == "" || !known[n.P] {
			roots = append(roots, n)
		}
	}
	roots = sortNodes(roots)
	if len(roots) == 1 && roots[0].Type == "folder" {
		roots = byParent[roots[0].H]
	}

	var lines []string
	var walk func(items []api.FileNode, depth int)
	walk = func(items []api.FileNode, depth int) {
		for _, n := range items {
			indent := strings.Repeat("  ", depth)
			name := n.Name
			if name == "" {
				name = n.H
			}
			if n.Type == "folder" {
				lines = append(lines, "  "+indent+name+"/")
				walk(byParent[n.H], depth+1)
				continue
			}
			lines = append(lines, "  "+indent+name+"\t"+format.Size(n.S))
		}
	}
	walk(roots, 0)
	return alignSizes(lines)
}

func sortNodes(nodes []api.FileNode) []api.FileNode {
	out := append([]api.FileNode(nil), nodes...)
	sort.Slice(out, func(i, j int) bool {
		iFolder := out[i].Type == "folder"
		jFolder := out[j].Type == "folder"
		if iFolder != jFolder {
			return iFolder
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func rootSet(nodes []api.FileNode) map[string]bool {
	known := map[string]bool{}
	for _, n := range nodes {
		known[n.H] = true
	}
	roots := map[string]bool{}
	for _, n := range nodes {
		if n.P == "" || !known[n.P] {
			roots[n.H] = true
		}
	}
	return roots
}

func alignSizes(lines []string) string {
	type row struct {
		name string
		size string
	}
	rows := make([]row, len(lines))
	width := 0
	for i, line := range lines {
		name, size, ok := strings.Cut(line, "\t")
		if !ok {
			rows[i] = row{name: line}
			continue
		}
		rows[i] = row{name: name, size: size}
		if n := len([]rune(name)); n > width {
			width = n
		}
	}
	if width > 48 {
		width = 48
	}
	var b strings.Builder
	for _, row := range rows {
		if row.size == "" {
			b.WriteString(row.name)
			b.WriteByte('\n')
			continue
		}
		name := row.name
		if len([]rune(name)) > width {
			name = string([]rune(name)[:width-1]) + "…"
		}
		gap := width - len([]rune(name))
		if gap < 2 {
			gap = 2
		}
		fmt.Fprintf(&b, "%s%s%s\n", name, strings.Repeat(" ", gap), row.size)
	}
	return b.String()
}
