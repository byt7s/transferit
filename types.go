package main

var (
	bt7API     = "https://bt7.api.mega.co.nz"
	gAPI       = "https://g.api.mega.co.nz"
	transferIT = "https://transfer.it"
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
)

type TransferInfo struct {
	Size []int64 `json:"size"`
	T    string  `json:"t"`
	M    string  `json:"m"`
	Z    string  `json:"z"`
}

type FileNode struct {
	H    string `json:"h"`
	T    int    `json:"t"`
	K    string `json:"k"`
	A    string `json:"a"`
	S    int64  `json:"s"`
	P    string `json:"p"`
	Name string `json:"-"`
	Type string `json:"-"`
}

type FileListResponse struct {
	F []FileNode `json:"f"`
}
