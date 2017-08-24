package content_view

import (
	"github.com/skycoin/bbs/src/store/object"
	"github.com/skycoin/skycoin/src/cipher"
	"log"
	"sync"
)

/*
	<<< BOARD >>>
*/

type BoardRep struct {
	PubKey       cipher.PubKey
	Name         string
	Body         string
	Created      int64
	SubAddresses []string
	Threads      []cipher.SHA256
}

func (r *BoardRep) Fill(pk cipher.PubKey, board *object.Board) *BoardRep {
	data := object.GetData(board)
	r.PubKey = pk
	r.Name = data.Name
	r.Body = data.Body
	r.Created = board.Created
	r.SubAddresses = data.SubAddresses
	return r
}

type BoardRepView struct {
	PubKey       string   `json:"public_key"`
	Name         string   `json:"name"`
	Body         string   `json:"body"`
	Created      int64    `json:"created"`
	SubAddresses []string `json:"submission_addresses"`
	ThreadCount  int      `json:"thread_count"`
}

func (r *BoardRep) View() *BoardRepView {
	if r == nil {
		return nil
	}
	return &BoardRepView{
		PubKey:       r.PubKey.Hex(),
		Name:         r.Name,
		Body:         r.Body,
		Created:      r.Created,
		SubAddresses: r.SubAddresses,
		ThreadCount:  len(r.Threads),
	}
}

/*
	<<< THREAD >>>
*/

type ThreadRep struct {
	Ref     cipher.SHA256
	Name    string
	Body    string
	Created int64
	Creator cipher.PubKey
	Posts   []cipher.SHA256
}

func (r *ThreadRep) FillThread(thread *object.Thread, mux *sync.Mutex) *ThreadRep {
	data := object.GetData(thread)
	r.Ref = thread.R
	r.Name = data.Name
	r.Body = data.Body
	r.Created = thread.Created
	r.Creator = thread.Creator
	return r
}

func (r *ThreadRep) FillThreadPage(tPage *object.ThreadPage, mux *sync.Mutex) *ThreadRep {
	t, e := tPage.GetThread(mux)
	if e != nil {
		log.Println("ThreadRep.FillThreadPage() Error:", e)
		return nil
	}
	data := object.GetData(t)
	r.Ref = t.R
	r.Name = data.Name
	r.Body = data.Body
	r.Created = t.Created
	r.Creator = t.Creator
	return r
}

type ThreadRepView struct {
	Ref       string       `json:"ref"`
	Name      string       `json:"name"`
	Body      string       `json:"body"`
	Created   int64        `json:"created"`
	Creator   string       `json:"creator"`
	Votes     *VoteRepView `json:"votes,omitempty"`
	PostCount int          `json:"post_count"`
}

func (r *ThreadRep) View(votes *VoteRepView) *ThreadRepView {
	if r == nil {
		return nil
	}
	return &ThreadRepView{
		Ref:       r.Ref.Hex(),
		Name:      r.Name,
		Body:      r.Body,
		Created:   r.Created,
		Creator:   r.Creator.Hex(),
		Votes:     votes,
		PostCount: len(r.Posts),
	}
}

/*
	<<< POST >>>
*/

type PostRep struct {
	Ref     cipher.SHA256
	Name    string
	Body    string
	Created int64
	Creator cipher.PubKey
}

func (r *PostRep) Fill(post *object.Post, mux *sync.Mutex) *PostRep {
	data := object.GetData(post)
	r.Ref = post.R
	r.Name = data.Name
	r.Body = data.Body
	r.Created = post.Created
	r.Creator = post.Creator
	return r
}

type PostRepView struct {
	Ref     string       `json:"ref"`
	Name    string       `json:"name"`
	Body    string       `json:"body"`
	Created int64        `json:"created"`
	Creator string       `json:"creator"`
	Votes   *VoteRepView `json:"votes,omitempty"`
}

func (r *PostRep) View(votes *VoteRepView) *PostRepView {
	if r == nil {
		return nil
	}
	return &PostRepView{
		Ref:     r.Ref.Hex(),
		Name:    r.Name,
		Body:    r.Body,
		Created: r.Created,
		Creator: r.Creator.Hex(),
		Votes:   votes,
	}
}

/*
	<<< VOTES >>>
*/

type VotesRep struct {
	Ref       cipher.SHA256
	Votes     map[cipher.PubKey]*object.Vote
	UpCount   int
	DownCount int
}

func (r *VotesRep) Fill(hash cipher.SHA256) *VotesRep {
	r.Ref = hash
	r.Votes = make(map[cipher.PubKey]*object.Vote)
	return r
}

func (r *VotesRep) Add(vote *object.Vote) {
	if oldVote, has := r.Votes[vote.Creator]; has {
		switch oldVote.Mode {
		case +1:
			r.UpCount--
		case -1:
			r.DownCount--
		}
	}
	r.Votes[vote.Creator] = vote
	switch vote.Mode {
	case +1:
		r.UpCount++
	case -1:
		r.DownCount++
	case 0:
		delete(r.Votes, vote.Creator)
	}
}

type X struct {
	Voted bool `json:"voted"`
	Count int  `json:"count"`
}

type VoteRepView struct {
	Ref  string `json:"ref"`
	Up   X      `json:"up_votes"`
	Down X      `json:"down_votes"`
}

func (r *VotesRep) View(perspective cipher.PubKey) *VoteRepView {
	if r == nil {
		return nil
	}
	vote := r.Votes[perspective]
	return &VoteRepView{
		Ref: r.Ref.Hex(),
		Up: X{
			Voted: vote != nil && vote.Mode == +1,
			Count: r.UpCount,
		},
		Down: X{
			Voted: vote != nil && vote.Mode == -1,
			Count: r.DownCount,
		},
	}
}