package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"time"
)

// ArchiveStorer defines an interface for interacting with an on disk
// versioned apt repository
type ArchiveStorer interface {
	ReleaseTags() map[string]StoreID
	GetReleaseTag(string) (StoreID, error)
	SetReleaseTag(string, StoreID) error
	DeleteReleaseTag(string) error

	//		AddDeb(file *ChangesItem) (*ReleaseIndexItem, error)
	AddControlFile(data ControlFile) (StoreID, error)
	GetControlFile(id StoreID) (ControlFile, error)

	GetReleaseRoot(seed Release) (StoreID, error)
	AddRelease(data *Release) (StoreID, error)
	GetRelease(id StoreID) (*Release, error)

	AddReleaseConfig(cfg ReleaseConfig) (StoreID, error)
	GetReleaseConfig(id StoreID) (ReleaseConfig, error)
	GetDefaultReleaseConfigID() (StoreID, error)

	EmptyReleaseIndex() (StoreID, error)
	AddReleaseIndex() (ReleaseIndexWriter, error)
	OpenReleaseIndex(id StoreID) (ReleaseIndexReader, error)

	GarbageCollect()
	DisableGarbageCollector()
	EnableGarbageCollector()
	Storer
}

type gcReq struct {
	done chan struct{}
}

type archiveBlobStore struct {
	Storer
	DefaultReleaseConfig ReleaseConfig
	enableGCChan         chan gcReq
	disableGCChan        chan gcReq
	runGCChan            chan gcReq
}

// NewArchiveBlobStore This creates a new repository, based ona a content
// addressable fs
func NewArchiveBlobStore(storeDir string, tmpDir string, defRelConfig ReleaseConfig) ArchiveStorer {
	result := &archiveBlobStore{
		Sha1Store(storeDir, tmpDir, 3),
		defRelConfig,
		make(chan gcReq),
		make(chan gcReq),
		make(chan gcReq),
	}

	go result.garbageCollector()

	return result
}

func (r archiveBlobStore) gcWalkReleaseConfig(used *SafeMap, id StoreID) {
	used.Set(id.String(), true)

	item, _ := r.GetReleaseConfig(id)

	used.Set(item.SigningKeyID.String(), true)
	for _, i := range item.PublicKeyIDs {
		used.Set(i.String(), true)
	}
}

func (r archiveBlobStore) gcWalkReleaseIndexEntryItem(used *SafeMap, item *ReleaseIndexEntryItem) {
	ctrlid := item.ControlID
	used.Set(ctrlid.String(), true)
	for _, f := range item.Files {
		used.Set(f.StoreID.String(), true)
	}
}

func (r archiveBlobStore) gcWalkReleaseIndex(used *SafeMap, id StoreID) {
	used.Set(StoreID(id).String(), true)
	index, _ := r.OpenReleaseIndex(id)
	for {
		entry, err := index.NextEntry()
		if err != nil {
			break
		}

		changesid := entry.ChangesID
		used.Set(changesid.String(), true)

		r.gcWalkReleaseIndexEntryItem(used, &entry.SourceItem)
		for _, item := range entry.BinaryItems {
			r.gcWalkReleaseIndexEntryItem(used, &item)
		}
	}
	index.Close()
}

func (r archiveBlobStore) gcWalkRelease(used *SafeMap, releaseID StoreID) {
	curr := releaseID
	trimmerActive := false
	trimAfter := int32(0)
	dropAssets := false

	for {
		if trimmerActive {
			if trimAfter > 0 {
				trimAfter--
			} else {
				// Stop makring assets after history trim
				dropAssets = true
			}
		}

		used.Set(curr.String(), true)
		release, _ := r.GetRelease(curr)
		if !dropAssets {
			used.Set(release.InRelease.String(), true)
			used.Set(release.Release.String(), true)
			used.Set(release.ReleaseGPG.String(), true)

			r.gcWalkReleaseConfig(used, release.ConfigID)

			for _, comp := range release.Components {
				used.Set(comp.SourcesGz.String(), true)
				for _, arch := range comp.Architectures {
					used.Set(arch.PackagesGz.String(), true)
				}
			}

			if !used.Check(StoreID(release.IndexID).String()) {
				r.gcWalkReleaseIndex(used, release.IndexID)
			}
		}

		if StoreID(release.ParentID).String() == r.EmptyFileID().String() {
			break
		}
		if used.Check(StoreID(release.ParentID).String()) {
			break
		}

		if release.TrimAfter > 0 && !trimmerActive {
			trimAfter = release.TrimAfter
			trimmerActive = true
		}
		curr = release.ParentID
	}
}

func (r archiveBlobStore) runGC() {
	log.Println("Beginning GC")

	stime := time.Now()
	gcFiles := 0
	gcBytes := int64(0)
	defer func() {
		gcDuration := time.Since(stime)
		log.Printf("GC %v files (%v bytes) in %v", gcFiles, gcBytes, gcDuration)
	}()

	used := NewSafeMap()
	refs := r.ListRefs()

	for _, id := range refs {
		r.gcWalkRelease(used, StoreID(id))
	}

	f := func(id StoreID) {
		if !used.Check(id.String()) {
			gcFiles++
			size, _ := r.Size(id)
			gcBytes += size
			log.Println("Removing unused blob ", id.String())
			r.UnLink(id)
		}
	}
	r.ForEach(f)
}

func (r archiveBlobStore) garbageCollector() {
	runGC := false
	lockCount := 0

	for {
		var req gcReq
		select {
		case req = <-r.enableGCChan:
			lockCount--
			if lockCount < 0 {
				panic("GC Lock count has gone negactive")
			}
		case req = <-r.disableGCChan:
			lockCount++
		case req = <-r.runGCChan:
			runGC = true
		}

		if runGC && lockCount == 0 {
			r.runGC()
			runGC = false
		}

		close(req.done)
	}
}

func (r archiveBlobStore) GarbageCollect() {
	c := make(chan struct{})
	r.runGCChan <- gcReq{c}
	<-c
}

func (r archiveBlobStore) DisableGarbageCollector() {
	log.Println("Disable GC")
	c := make(chan struct{})
	r.disableGCChan <- gcReq{c}
	<-c
}

func (r archiveBlobStore) EnableGarbageCollector() {
	log.Println("Enable GC")
	c := make(chan struct{})
	r.enableGCChan <- gcReq{c}
	<-c
}

func (r archiveBlobStore) GetReleaseTag(name string) (StoreID, error) {
	return r.GetRef(name)
}

func (r archiveBlobStore) SetReleaseTag(name string, id StoreID) error {
	return r.SetRef(name, StoreID(id))
}

func (r archiveBlobStore) DeleteReleaseTag(name string) error {
	return r.DeleteRef(name)
}

/*
func (r archiveBlobStore) AddDeb(file *ChangesItem) (*ReleaseIndexItem, error) {
		var item ReleaseIndexItem
		item.Type = BINARY

		pkgReader, err := r.Open(file.StoreID)
		if err != nil {
			return nil, err
		}
		defer pkgReader.Close()

		pkg := NewDebPackage(pkgReader, nil)
		err = pkg.Parse()
		if err != nil {
			return nil, err
		}

		control, _ := pkg.Control()
		arch, ok := control.GetValue("Architecture")
		if !ok {
			arch = "all"
		}
		item.Architecture = arch

		control.SetValue("Filename", file.Filename)
		control.SetValue("Size", strconv.FormatInt(file.Size, 10))
		control.SetValue("MD5sum", file.Md5)
		control.SetValue("SHA1", file.Sha1)
		control.SetValue("SHA256", file.Sha256)

		paragraphs := make(ControlFile, 1)
		paragraphs[0] = control

		item.ControlID, err = r.AddBinaryControlFile(paragraphs)
		if err != nil {
			return nil, err
		}

		item.Name, _ = pkg.Name()
		item.Version, _ = pkg.Version()

		fileSlice := make([]ReleaseIndexItemFile, 1)
		fileSlice[0] = ReleaseIndexItemFile{
			Name: file.Filename,
			ID:   file.StoreID,
		}
		item.Files = fileSlice

		return &item, nil
}
*/

// We can't serialize a map, as the key order is not
// guaranteed, which will result in inconsistant
// hashes for the same data.
type consistantControlFileParagraph struct {
	Keys   []string
	Values [][]string
}

type consistantControlFile struct {
	Paragraphs        []consistantControlFileParagraph
	SignedBy          []string
	Signed            bool
	SignatureVerified bool
	Original          StoreID
}

func (r archiveBlobStore) GetControlFile(id StoreID) (ControlFile, error) {
	reader, err := r.Open(id)
	if err != nil {
		return ControlFile{}, err
	}
	defer reader.Close()

	dec := gob.NewDecoder(reader)
	var item consistantControlFile
	err = dec.Decode(&item)
	if err != nil {
		return ControlFile{}, err
	}

	result := ControlFile{
		SignatureVerified: item.SignatureVerified,
		SignedBy:          item.SignedBy,
		Signed:            item.Signed,
	}

	result.Data = make([]*ControlParagraph, len(item.Paragraphs))
	for i := range item.Paragraphs {
		para := MakeControlParagraph()
		result.Data[i] = &para
		for j := range item.Paragraphs[i].Keys {
			strVals := item.Paragraphs[i].Values[j]
			for k := range strVals {
				result.Data[i].AddValue(item.Paragraphs[i].Keys[j], strVals[k])
			}
		}
	}

	return result, nil
}

func (r archiveBlobStore) AddControlFile(item ControlFile) (StoreID, error) {
	data := make([]consistantControlFileParagraph, len(item.Data))

	for i := range item.Data {
		data[i].Keys = make([]string, len(*item.Data[i]))
		data[i].Values = make([][]string, len(*item.Data[i]))

		j := 0
		for s := range *item.Data[i] {
			data[i].Keys[j] = s
			j++
		}
		sort.Strings(data[i].Keys)

		for j = range data[i].Keys {
			key := data[i].Keys[j]
			valptrs, _ := item.Data[i].GetValues(key)
			vals := make([]string, len(valptrs))
			for k := range valptrs {
				vals[k] = *valptrs[k]
			}
			data[i].Values[j] = vals
		}
	}

	file := consistantControlFile{
		SignedBy:          item.SignedBy,
		Signed:            item.Signed,
		SignatureVerified: item.SignatureVerified,
		Paragraphs:        data,
	}

	writer, err := r.Store()
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)

	err = enc.Encode(file)
	if err != nil {
		return nil, err
	}

	writer.Close()
	id, err := writer.Identity()
	if err != nil {
		return nil, err
	}

	return id, nil
}

func (r archiveBlobStore) EmptyReleaseIndex() (id StoreID, err error) {
	idx, err := r.AddReleaseIndex()
	return idx.Close()
}

// RepoReleaseIndex represent a complete list of packages that will make
// up a full release.
func (r archiveBlobStore) AddReleaseIndex() (ReleaseIndexWriter, error) {
	var h repoReleaseIndexWriterHandle
	var err error
	h.handle, err = r.Store()
	if err != nil {
		return nil, err
	}

	h.encoder = gob.NewEncoder(h.handle)
	return &h, err
}

// Used for tracking the state of reads from an ReleaseIndex
type repoReleaseIndexWriterHandle struct {
	handle  StoreWriteCloser
	encoder *gob.Encoder
}

func (r *repoReleaseIndexWriterHandle) AddEntry(item *ReleaseIndexEntry) (err error) {
	err = r.encoder.Encode(item)
	return
}

func (r *repoReleaseIndexWriterHandle) Close() (StoreID, error) {
	err := r.handle.Close()
	if err != nil {
		return nil, err
	}

	id, err := r.handle.Identity()
	return id, err
}

type repoReleaseIndexReaderHandle struct {
	handle  io.ReadCloser
	decoder *gob.Decoder
}

func (r archiveBlobStore) OpenReleaseIndex(id StoreID) (ReleaseIndexReader, error) {
	var h repoReleaseIndexReaderHandle
	var err error

	h.handle, err = r.Open(id)
	if err != nil {
		return nil, err
	}

	h.decoder = gob.NewDecoder(h.handle)
	return &h, err
}

func (r *repoReleaseIndexReaderHandle) NextEntry() (item ReleaseIndexEntry, err error) {
	err = r.decoder.Decode(&item)
	return
}

func (r *repoReleaseIndexReaderHandle) Close() error {
	err := r.handle.Close()
	return err
}

func (r archiveBlobStore) GetRelease(id StoreID) (*Release, error) {
	var rel Release
	reader, err := r.Open(id)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	dec := gob.NewDecoder(reader)
	err = dec.Decode(&rel)
	if err != nil {
		return nil, fmt.Errorf("reading release object failed, %v", err)
	}

	rel.store = r
	rel.id = id

	return &rel, nil
}

// GetReleaseRags  Get all of the refs related to a relesse
func (r archiveBlobStore) ReleaseTags() map[string]StoreID {
	return r.ListRefs()
}

// GetReleaseRoot returns an ID suitable for use as the parent ID for a new
// release
func (r archiveBlobStore) GetReleaseRoot(seed Release) (StoreID, error) {
	var err error

	seed.ParentID = r.EmptyFileID()
	seed.Date = time.Now()
	seed.Actions = []ReleaseLogAction{}

	seed.ConfigID, err = r.GetDefaultReleaseConfigID()
	if err != nil {
		return nil, errors.New("getting default config failed, " + err.Error())
	}

	seed.IndexID, err = r.EmptyReleaseIndex()
	if err != nil {
		return nil, errors.New("creating empty index failed, " + err.Error())
	}

	id, err := r.AddRelease(&seed)
	if err != nil {
		return nil, errors.New("creating empty index failed, " + err.Error())
	}

	r.SetReleaseTag("heads/"+seed.CodeName, id)
	log.Println("Initialised new distribution " + seed.Suite)

	return id, nil
}

func (r archiveBlobStore) AddRelease(data *Release) (StoreID, error) {
	writer, err := r.Store()
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)

	enc.Encode(data)
	writer.Close()
	id, err := writer.Identity()
	if err != nil {
		return nil, err
	}

	return StoreID(id), nil
}

// GetReleaseConfig returns the release configuration stored in the given blob
func (r archiveBlobStore) GetDefaultReleaseConfigID() (StoreID, error) {
	return r.AddReleaseConfig(r.DefaultReleaseConfig)
}

// GetReleaseConfig returns the release configuration stored in the given blob
func (r archiveBlobStore) GetReleaseConfig(id StoreID) (ReleaseConfig, error) {
	var cfg ReleaseConfig
	reader, err := r.Open(id)
	if err != nil {
		return ReleaseConfig{}, err
	}
	defer reader.Close()

	dec := gob.NewDecoder(reader)
	dec.Decode(&cfg)

	return cfg, nil
}

// AddReleaseConfig stored a ReleaseConfig in the blob store, and returns the ID
func (r archiveBlobStore) AddReleaseConfig(cfg ReleaseConfig) (StoreID, error) {
	writer, err := r.Store()
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)

	err = enc.Encode(cfg)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	id, err := writer.Identity()
	if err != nil {
		return nil, err
	}

	return StoreID(id), nil
}
