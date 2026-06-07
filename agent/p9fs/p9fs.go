package p9fs

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/knusbaum/go9p"
	"github.com/knusbaum/go9p/fs"
	"github.com/knusbaum/go9p/proto"
)

// NewFS builds the RAG 9P file tree backed by svc.
func NewFS(svc Service) (*fs.FS, error) {
	p9, root := fs.NewFS("rag", "rag", 0555,
		fs.IgnorePermissions(),
		fs.WithCreateFile(func(filesystem *fs.FS, parent fs.Dir, user, name string, perm uint32, mode uint8) (fs.File, error) {
			if parent.Stat().Name != "documents" {
				return nil, fmt.Errorf("create only supported under /documents")
			}
			modParent, ok := parent.(fs.ModDir)
			if !ok {
				return nil, fmt.Errorf("documents does not support create")
			}
			f := fs.NewStaticFile(filesystem.NewStat(name, user, user, 0644), []byte{})
			if err := modParent.AddChild(f); err != nil {
				return nil, err
			}
			return f, nil
		}),
		fs.WithRemoveFile(func(filesystem *fs.FS, node fs.FSNode) error {
			path := fs.FullPath(node)
			if !strings.HasPrefix(path, "/documents/") || path == "/documents" {
				return fmt.Errorf("remove only supported under /documents/<doc_id>")
			}
			docID := node.Stat().Name
			if err := svc.DeleteDocument(docID); err != nil {
				return err
			}
			parent, ok := node.Parent().(fs.ModDir)
			if !ok {
				return fmt.Errorf("parent does not support modification")
			}
			return parent.DeleteChild(docID)
		}),
	)

	retrieveArea := &opArea{params: make(map[string]string)}
	searchArea := &opArea{params: make(map[string]string)}

	_ = root.AddChild(fs.NewStaticFile(p9.NewStat("README", "rag", "rag", 0444), []byte(readmeText)))
	_ = root.AddChild(newCtlFile(p9.NewStat("ctl", "rag", "rag", 0644), svc))
	_ = root.AddChild(newIngestFile(p9.NewStat("ingest", "rag", "rag", 0644), svc))
	_ = root.AddChild(fs.NewDynamicFile(p9.NewStat("stats", "rag", "rag", 0444), func() []byte {
		data, err := svc.StatsJSON()
		if err != nil {
			return []byte("error: " + err.Error() + "\n")
		}
		return data
	}))

	retrieveDir := fs.NewStaticDir(p9.NewStat("retrieve", "rag", "rag", 0555|proto.DMDIR))
	_ = retrieveDir.AddChild(newQueryCtlFile(p9.NewStat("ctl", "rag", "rag", 0644), retrieveArea))
	_ = retrieveDir.AddChild(newParamsFile(p9.NewStat("params", "rag", "rag", 0644), retrieveArea))
	_ = retrieveDir.AddChild(newRetrieveDataFile(p9.NewStat("data", "rag", "rag", 0444), retrieveArea, svc))
	_ = root.AddChild(retrieveDir)

	searchDir := fs.NewStaticDir(p9.NewStat("search", "rag", "rag", 0555|proto.DMDIR))
	_ = searchDir.AddChild(newQueryCtlFile(p9.NewStat("ctl", "rag", "rag", 0644), searchArea))
	_ = searchDir.AddChild(newParamsFile(p9.NewStat("params", "rag", "rag", 0644), searchArea))
	_ = searchDir.AddChild(newSearchDataFile(p9.NewStat("data", "rag", "rag", 0444), searchArea, svc))
	_ = searchDir.AddChild(newSearchMetaFile(p9.NewStat("metadata", "rag", "rag", 0444), searchArea, svc))
	_ = root.AddChild(searchDir)

	documentsDir := fs.NewStaticDir(p9.NewStat("documents", "rag", "rag", 0777|proto.DMDIR))
	_ = root.AddChild(documentsDir)

	return p9, nil
}

// Serve starts a 9P listener on addr.
// Supported forms: tcp!host!port, unix!/path/to/socket, or host:port.
func Serve(addr string, svc Service) error {
	p9fs, err := NewFS(svc)
	if err != nil {
		return err
	}
	srv := p9fs.Server()

	network, address, err := parsePlan9Addr(addr)
	if err != nil {
		return err
	}

	if network == "unix" {
		if err := os.MkdirAll(filepath.Dir(address), 0o755); err != nil {
			return err
		}
		_ = os.Remove(address)
	}

	go func() {
		var ln net.Listener
		var listenErr error
		if network == "unix" {
			ln, listenErr = net.Listen("unix", address)
		} else {
			ln, listenErr = net.Listen("tcp", address)
		}
		if listenErr != nil {
			log.Printf("9p listen error: %v", listenErr)
			return
		}
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("9p accept error: %v", err)
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				read := bufio.NewReader(c)
				if err := go9p.ServeReadWriter(read, c, srv); err != nil {
					log.Printf("9p session error: %v", err)
				}
			}(conn)
		}
	}()

	if network == "unix" {
		log.Printf("9p serving on unix!%s", address)
	} else {
		log.Printf("9p serving on tcp!%s", address)
	}
	return nil
}

func parsePlan9Addr(addr string) (network, address string, err error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", "", fmt.Errorf("empty 9p address")
	}
	if strings.Contains(addr, "!") {
		parts := strings.SplitN(addr, "!", 3)
		switch len(parts) {
		case 2:
			return parts[0], parts[1], nil
		case 3:
			if parts[0] == "tcp" {
				return "tcp", net.JoinHostPort(parts[1], parts[2]), nil
			}
			return parts[0], strings.Join(parts[1:], "!"), nil
		}
	}
	return "tcp", addr, nil
}
