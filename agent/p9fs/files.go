package p9fs

import (
	"fmt"
	"strings"
	"sync"

	"github.com/knusbaum/go9p/fs"
	"github.com/knusbaum/go9p/proto"
)

const readmeText = `RAG agent 9P file tree

Top-level:
  README   this file
  stats    read JSON index stats
  ctl      write: reset | finalize | status
  ingest   write JSON document; read last result

search/
  ctl      write question (q)
  params   read/write key=value retrieval options
  data     read answer text after writing ctl
  metadata read JSON prompt/model metadata after search

retrieve/
  ctl      write retrieval query
  params   read/write key=value options
  data     read JSON hits after writing ctl

documents/
  <doc_id> create then remove to delete a document
`

type opArea struct {
	mu      sync.Mutex
	query   string
	params  map[string]string
	result  []byte
	meta    []byte
	errText string
}

func (a *opArea) setQuery(q string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.query = strings.TrimSpace(q)
	a.result = nil
	a.meta = nil
	a.errText = ""
}

func (a *opArea) setParams(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.params = parseParamsText(text)
}

func (a *opArea) paramsText() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return formatParamsText(a.params)
}

func (a *opArea) getQuery() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.query
}

func (a *opArea) getParams() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]string, len(a.params))
	for k, v := range a.params {
		out[k] = v
	}
	return out
}

func (a *opArea) setResult(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.result = data
	a.errText = ""
}

func (a *opArea) setMeta(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.meta = data
}

func (a *opArea) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.errText = err.Error()
	a.result = nil
}

func (a *opArea) readMeta() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.errText != "" {
		return nil, fmt.Errorf("%s", a.errText)
	}
	return append([]byte(nil), a.meta...), nil
}

type ctlFile struct {
	fs.BaseFile
	svc    Service
	result []byte
}

func newCtlFile(stat *proto.Stat, svc Service) *ctlFile {
	return &ctlFile{BaseFile: *fs.NewBaseFile(stat), svc: svc}
}

func (f *ctlFile) Write(_ uint64, _ uint64, data []byte) (uint32, error) {
	cmd := strings.TrimSpace(string(data))
	result, err := f.svc.RunCtl(cmd)
	if err != nil {
		return 0, err
	}
	f.result = []byte(result + "\n")
	return uint32(len(data)), nil
}

func (f *ctlFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	if len(f.result) == 0 {
		status, err := f.svc.RunCtl("status")
		if err != nil {
			return nil, err
		}
		f.result = []byte(status + "\n")
	}
	return readSlice(f.result, offset, count), nil
}

type ingestFile struct {
	fs.BaseFile
	svc    Service
	result []byte
}

func newIngestFile(stat *proto.Stat, svc Service) *ingestFile {
	return &ingestFile{BaseFile: *fs.NewBaseFile(stat), svc: svc}
}

func (f *ingestFile) Write(_ uint64, _ uint64, data []byte) (uint32, error) {
	result, err := f.svc.IngestJSON(data)
	if err != nil {
		f.result = []byte("error: " + err.Error() + "\n")
		return 0, err
	}
	f.result = []byte(result + "\n")
	return uint32(len(data)), nil
}

func (f *ingestFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	return readSlice(f.result, offset, count), nil
}

type queryCtlFile struct {
	fs.BaseFile
	area *opArea
}

func newQueryCtlFile(stat *proto.Stat, area *opArea) *queryCtlFile {
	return &queryCtlFile{BaseFile: *fs.NewBaseFile(stat), area: area}
}

func (f *queryCtlFile) Write(_ uint64, _ uint64, data []byte) (uint32, error) {
	f.area.setQuery(string(data))
	return uint32(len(data)), nil
}

func (f *queryCtlFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	return readSlice([]byte(f.area.getQuery()), offset, count), nil
}

type paramsFile struct {
	fs.BaseFile
	area *opArea
}

func newParamsFile(stat *proto.Stat, area *opArea) *paramsFile {
	return &paramsFile{BaseFile: *fs.NewBaseFile(stat), area: area}
}

func (f *paramsFile) Write(_ uint64, _ uint64, data []byte) (uint32, error) {
	f.area.setParams(string(data))
	return uint32(len(data)), nil
}

func (f *paramsFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	return readSlice([]byte(f.area.paramsText()), offset, count), nil
}

type retrieveDataFile struct {
	fs.BaseFile
	area *opArea
	svc  Service
}

func newRetrieveDataFile(stat *proto.Stat, area *opArea, svc Service) *retrieveDataFile {
	return &retrieveDataFile{BaseFile: *fs.NewBaseFile(stat), area: area, svc: svc}
}

func (f *retrieveDataFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	query := f.area.getQuery()
	if query == "" {
		return nil, fmt.Errorf("write a query to retrieve/ctl first")
	}
	data, err := f.svc.Retrieve(query, f.area.getParams())
	if err != nil {
		f.area.setError(err)
		return nil, err
	}
	f.area.setResult(data)
	return readSlice(data, offset, count), nil
}

type searchDataFile struct {
	fs.BaseFile
	area *opArea
	svc  Service
}

func newSearchDataFile(stat *proto.Stat, area *opArea, svc Service) *searchDataFile {
	return &searchDataFile{BaseFile: *fs.NewBaseFile(stat), area: area, svc: svc}
}

func (f *searchDataFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	query := f.area.getQuery()
	if query == "" {
		return nil, fmt.Errorf("write a question to search/ctl first")
	}
	answer, meta, err := f.svc.Search(query, f.area.getParams())
	if err != nil {
		f.area.setError(err)
		return nil, err
	}
	f.area.setResult([]byte(answer))
	f.area.setMeta(meta)
	return readSlice([]byte(answer), offset, count), nil
}

type searchMetaFile struct {
	fs.BaseFile
	area *opArea
	svc  Service
}

func newSearchMetaFile(stat *proto.Stat, area *opArea, svc Service) *searchMetaFile {
	return &searchMetaFile{BaseFile: *fs.NewBaseFile(stat), area: area, svc: svc}
}

func (f *searchMetaFile) Read(_ uint64, offset uint64, count uint64) ([]byte, error) {
	if data, err := f.area.readMeta(); err == nil && len(data) > 0 {
		return readSlice(data, offset, count), nil
	}
	query := f.area.getQuery()
	if query == "" {
		return nil, fmt.Errorf("write a question to search/ctl first")
	}
	_, meta, err := f.svc.Search(query, f.area.getParams())
	if err != nil {
		return nil, err
	}
	f.area.setMeta(meta)
	return readSlice(meta, offset, count), nil
}

func readSlice(data []byte, offset, count uint64) []byte {
	if offset >= uint64(len(data)) {
		return []byte{}
	}
	end := offset + count
	if end > uint64(len(data)) {
		end = uint64(len(data))
	}
	return data[offset:end]
}
