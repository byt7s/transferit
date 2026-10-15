package download

import (
	"fmt"

	"transferit/internal/api"
	"transferit/internal/crypto"
)

func (d *Downloader) decodeNodes(nodes []api.FileNode) []api.FileNode {
	decoded := make([]api.FileNode, 0, len(nodes))
	for _, n := range nodes {
		if n.T == 1 {
			n.Type = "folder"
		} else {
			n.Type = "file"
		}

		if n.K != "" && n.A != "" {
			keyA32, err := crypto.Base64ToA32(n.K)
			if err != nil {
				n.Name = fmt.Sprintf("[key-error-%s]", n.H)
				decoded = append(decoded, n)
				continue
			}

			if len(keyA32) == 4 {
				keyA32 = append(keyA32, keyA32...)
			}

			if len(keyA32) >= 8 {
				attrs, err := crypto.DecryptAttr(n.A, keyA32[:8])
				if err == nil {
					if name, ok := attrs["n"].(string); ok {
						n.Name = name
					} else {
						n.Name = fmt.Sprintf("[encrypted-%s]", n.H)
					}
				} else {
					n.Name = fmt.Sprintf("[encrypted-%s]", n.H)
				}
			} else {
				n.Name = fmt.Sprintf("[short-key-%s]", n.H)
			}
		} else if n.Name == "" {
			n.Name = fmt.Sprintf("[no-attrs-%s]", n.H)
		}

		decoded = append(decoded, n)
	}
	return decoded
}
