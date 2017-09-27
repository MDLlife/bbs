package setup

import (
	"github.com/skycoin/cxo/skyobject"
	"github.com/skycoin/bbs/src/store/object/revisions/r0"
	"github.com/skycoin/bbs/src/store/object"
	"github.com/skycoin/cxo/node"
)

// PrepareRegistry sets up the CXO Registry.
func PrepareRegistry(r *skyobject.Reg) {
	r.Register(r0.RootPageName, r0.RootPage{})
	r.Register(r0.BoardPageName, r0.BoardPage{})
	r.Register(r0.ThreadPageName, r0.ThreadPage{})
	r.Register(r0.DiffPageName, r0.DiffPage{})
	r.Register(r0.UsersPageName, r0.UsersPage{})
	r.Register(r0.UserActivityPageName, r0.UserActivityPage{})
	r.Register(r0.BoardName, r0.Board{})
	r.Register(r0.ThreadName, r0.Thread{})
	r.Register(r0.PostName, r0.Post{})
	r.Register(r0.VoteName, r0.Vote{})
	r.Register(r0.UserName, r0.User{})
}

// NewBoard generates a new board.
func NewBoard(node *node.Node, in *object.NewBoardIO) (*skyobject.Root, error) {
	pack, e := node.Container().NewRoot(
		in.BoardPubKey,
		in.BoardSecKey,
		skyobject.HashTableIndex|skyobject.EntireTree,
		node.Container().CoreRegistry().Types(),
	)
	if e != nil {
		return nil, e
	}

	pack.Append(
		&r0.RootPage{
			Typ: r0.RootTypeBoard,
			Rev: 0,
			Del: false,
			Sum: in.Board.Raw,
		},
		&r0.BoardPage{
			Board: pack.Ref(in.Board),
		},
		&r0.DiffPage{},
		&r0.UsersPage{},
	)
	if e := pack.Save(); e != nil {
		return nil, e
	}
	node.Publish(pack.Root())
	pack.Close()

	return node.Container().LastRoot(in.BoardPubKey)
}