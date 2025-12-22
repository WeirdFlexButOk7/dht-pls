package protocols

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	// FileTransferProtocol is the protocol ID for file transfers
	FileTransferProtocol protocol.ID = "/dht-p2p/file-transfer/1.0.0"
)

// FileRequest represents a file transfer request
type FileRequest struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	ChunkIdx int    `json:"chunk_idx"`
}

// FileChunk represents a chunk of file data
type FileChunk struct {
	FileName string `json:"file_name"`
	ChunkIdx int    `json:"chunk_idx"`
	Data     []byte `json:"data"`
	IsLast   bool   `json:"is_last"`
}

// FileTransferHandler handles file transfer operations
type FileTransferHandler struct {
	host          host.Host
	onFileReceive func(fileName string, data []byte)
}

// NewFileTransferHandler creates a new file transfer handler
func NewFileTransferHandler(h host.Host, onReceive func(string, []byte)) *FileTransferHandler {
	handler := &FileTransferHandler{
		host:          h,
		onFileReceive: onReceive,
	}

	h.SetStreamHandler(FileTransferProtocol, handler.handleStream)
	return handler
}

// handleStream handles incoming file transfer streams
func (fth *FileTransferHandler) handleStream(s network.Stream) {
	defer s.Close()

	reader := bufio.NewReader(s)
	var chunks []FileChunk

	for {
		s.SetReadDeadline(time.Now().Add(30 * time.Second))
		data, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Error reading file chunk: %v\n", err)
			}
			break
		}

		var chunk FileChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			fmt.Printf("Error unmarshaling chunk: %v\n", err)
			continue
		}

		chunks = append(chunks, chunk)

		if chunk.IsLast {
			// Reassemble file
			var fileData []byte
			for _, c := range chunks {
				fileData = append(fileData, c.Data...)
			}

			if fth.onFileReceive != nil {
				fth.onFileReceive(chunk.FileName, fileData)
			}

			// Send acknowledgment
			ack := map[string]string{"status": "received", "file": chunk.FileName}
			ackBytes, _ := json.Marshal(ack)
			s.Write(append(ackBytes, '\n'))
			break
		}
	}
}

// SendFile sends a file to a specific peer in chunks
func (fth *FileTransferHandler) SendFile(ctx context.Context, peerID peer.ID, fileName string, data []byte) error {
	s, err := fth.host.NewStream(ctx, peerID, FileTransferProtocol)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer s.Close()

	// Send file in chunks of 64KB
	chunkSize := 64 * 1024
	writer := bufio.NewWriter(s)

	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		isLast := false
		if end >= len(data) {
			end = len(data)
			isLast = true
		}

		chunk := FileChunk{
			FileName: fileName,
			ChunkIdx: i / chunkSize,
			Data:     data[i:end],
			IsLast:   isLast,
		}

		chunkBytes, err := json.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("failed to marshal chunk: %w", err)
		}

		s.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := writer.Write(append(chunkBytes, '\n')); err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("failed to flush chunk: %w", err)
		}
	}

	// Read acknowledgment
	s.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(s)
	ackBytes, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read ack: %w", err)
	}

	var ack map[string]string
	if err := json.Unmarshal(ackBytes, &ack); err == nil {
		if ack["status"] == "received" {
			fmt.Printf("✅ File %s sent successfully\n", fileName)
			return nil
		}
	}

	return nil
}
