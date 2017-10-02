package content_view

import (
	"github.com/skycoin/bbs/src/store/object/revisions/r0"
	"github.com/skycoin/bbs/src/store/state/pack"
	"github.com/skycoin/cxo/skyobject"
	"github.com/skycoin/skycoin/src/cipher"
	"sync"
)

type indexPage struct {
	Board   string
	Threads []string            // Board threads.
	Posts   map[string][]string // Key: thread hashes, Value: post hash array.
}

func newIndexPage() *indexPage {
	return &indexPage{
		Posts: make(map[string][]string),
	}
}

type ContentView struct {
	sync.Mutex
	pk cipher.PubKey
	i  *indexPage
	c  map[string]*r0.ContentRep
	v  map[string]*VotesRep
}

func (v *ContentView) Init(pack *skyobject.Pack, headers *pack.Headers) error {
	v.Lock()
	defer v.Unlock()

	pages, e := r0.GetPages(pack, false, true, false, true)
	if e != nil {
		return e
	}

	v.pk = pack.Root().Pub
	v.i = newIndexPage()
	v.c = make(map[string]*r0.ContentRep)
	v.v = make(map[string]*VotesRep)

	// Set board.
	board, e := pages.BoardPage.GetBoard()
	if e != nil {
		return e
	}
	v.i.Board = board.GetHeader().Hash
	v.c[v.i.Board] = board.ToRep()

	v.i.Threads = make([]string, pages.BoardPage.GetThreadCount())

	// Fill threads and posts.
	e = pages.BoardPage.RangeThreadPages(func(i int, tp *r0.ThreadPage) error {

		thread, e := tp.GetThread()
		if e != nil {
			return e
		}
		threadHash := thread.GetHeader().Hash

		v.i.Threads[i] = threadHash
		v.c[threadHash] = thread.ToRep()

		// Fill posts.
		postHashes := make([]string, tp.GetPostCount())
		e = tp.RangePosts(func(i int, post *r0.Post) error {
			postHashes[i] = post.GetHeader().Hash
			v.c[postHashes[i]] = post.ToRep()
			return nil
		})
		if e != nil {
			return e
		}
		v.i.Posts[threadHash] = postHashes
		return nil
	})

	if e != nil {
		return e
	}

	return pages.UsersPage.RangeUserProfiles(func(_ int, uap *r0.UserProfile) error {
		return uap.RangeSubmissions(func(_ int, c *r0.Content) error {
			return v.processVote(c)
		})
	})
}

func (v *ContentView) Update(pack *skyobject.Pack, headers *pack.Headers) error {
	v.Lock()
	defer v.Unlock()

	pages, e := r0.GetPages(pack, false, true)
	if e != nil {
		return e
	}
	board, e := pages.BoardPage.GetBoard()
	if e != nil {
		return e
	}
	delete(v.c, v.i.Board)
	v.i.Board = board.GetHeader().Hash
	v.c[v.i.Board] = board.ToRep()

	changes := headers.GetChanges()

	for _, content := range changes.New {
		header := content.GetHeader()
		switch header.Type {
		case r0.V5ThreadType:
			thread := content.ToThread()
			v.i.Threads = append(v.i.Threads, header.Hash)
			v.c[header.Hash] = thread.ToRep()

		case r0.V5PostType:
			post := content.ToPost()
			postBody := post.GetBody()
			posts, _ := v.i.Posts[postBody.OfThread]
			v.i.Posts[postBody.OfThread] = append(posts, header.Hash)
			v.c[header.Hash] = post.ToRep()

		case r0.V5ThreadVoteType, r0.V5PostVoteType:
			v.processVote(content)
		}
	}
	return nil
}

func (v *ContentView) processVote(c *r0.Content) error {
	var cHash string
	var cType r0.ContentType

	// Only if vote is for post or thread.
	switch c.GetHeader().Type {
	case r0.V5ThreadVoteType:
		if v.c[c.ToThreadVote().GetBody().OfThread] == nil {
			return nil
		}
		cHash = c.GetHeader().Hash
		cType = r0.V5ThreadVoteType
	case r0.V5PostVoteType:
		if v.c[c.ToPostVote().GetBody().OfPost] == nil {
			return nil
		}
		cHash = c.GetHeader().Hash
		cType = r0.V5PostVoteType
	case r0.V5UserVoteType:
		return nil
	default:
		return nil
	}

	// Add to votes map.
	voteRep, has := v.v[cHash]
	if !has {
		voteRep = new(VotesRep).Fill(cType, cHash)
		v.v[cHash] = voteRep
	}
	voteRep.Add(c)

	return nil
}
