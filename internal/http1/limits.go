package http1

const (
	maxRequestLineBytes = 8 << 10 // 8 KiB
	maxHeaderLineBytes  = 8 << 10
	maxHeaderLines      = 100
	maxBodyBytes        = 10 << 20  // 10 MiB
	maxDrainBytes       = 256 << 10 // 256 KiB
	maxChunkSize        = 1 << 31
)
