package state

import (
	"context"
	"github.com/skycoin/bbs/src/misc/boo"
	"github.com/skycoin/bbs/src/misc/inform"
	"github.com/skycoin/bbs/src/store/object"
	"github.com/skycoin/bbs/src/store/state/pack"
	"github.com/skycoin/bbs/src/store/state/views"
	"github.com/skycoin/cxo/node"
	"github.com/skycoin/cxo/skyobject"
	"github.com/skycoin/skycoin/src/cipher"
	"log"
	"os"
	"sync"
	"time"
	"fmt"
)

type BoardInstanceConfig struct {
	Master bool
	PK     cipher.PubKey
	SK     cipher.SecKey
}

type BoardInstance struct {
	c *BoardInstanceConfig
	l *log.Logger

	flag skyobject.Flag // Used for compiling pack.

	piMux sync.Mutex
	pi    *PackInstance
	views map[string]views.View

	changesChan chan *object.Changes // Changes to tree (for output - web socket).

	needUpdateMux sync.RWMutex
	needUpdate    bool
}

func NewBoardInstance(
	config *BoardInstanceConfig,
	ct *skyobject.Container,
	root *skyobject.Root,
	viewAdders ...views.Adder,
) (
	*BoardInstance, error,
) {
	// Prepare output.
	bi := &BoardInstance{
		c:           config,
		l:           inform.NewLogger(true, os.Stdout, "INSTANCE:"+config.PK.Hex()),
		views:       make(map[string]views.View),
		changesChan: make(chan *object.Changes, 10),
	}

	// Prepare flags.
	bi.flag = skyobject.HashTableIndex | skyobject.EntireTree
	if !bi.c.Master {
		bi.flag |= skyobject.ViewOnly
	}

	// Prepare pack instance.
	p, e := ct.Unpack(root, bi.flag, ct.CoreRegistry().Types(), config.SK)
	if e != nil {
		return nil, e
	}
	bi.pi, e = NewPackInstance(nil, p)
	if e != nil {
		return nil, e
	}

	// Initiate views.
	for _, adder := range viewAdders {
		views.Add(bi.views, adder)
	}
	for _, view := range bi.views {
		if e := view.Init(p, bi.pi.headers, nil); e != nil {
			return nil, boo.WrapType(e, boo.Internal,
				"failed to generate view")
		}
		fmt.Println("VIEW LOADED:", view)
	}

	// Output.
	return bi, nil
}

// Update updates the board instance. (External trigger).
func (bi *BoardInstance) Update(node *node.Node, root *skyobject.Root) error {
	return bi.ChangePack(func(oldPI *PackInstance) (*PackInstance, error) {

		if root == nil {
			// If master of board, update last changes.
			if bi.c.Master {
				e := oldPI.Do(func(p *skyobject.Pack, h *pack.Headers) error {
					_, e := p.Save()
					if e != nil {
						return e
					}
					node.Publish(p.Root())
					root, e = node.Container().LastFull(bi.c.PK)
					return e
				})
				if e != nil {
					return nil, e
				}
			} else {
				return oldPI, nil
			}
		}

		// Prepare new pack instance.
		ct := node.Container()
		newPack, e := ct.Unpack(root, bi.flag, ct.CoreRegistry().Types(), bi.c.SK)
		if e != nil {
			return nil, e
		}
		newPI, e := NewPackInstance(oldPI, newPack)
		if e != nil {
			return nil, e
		}

		// Update views.
		for _, view := range bi.views {
			if e := view.Update(newPI.pack, newPI.headers, nil); e != nil {
				return nil, boo.WrapType(e, boo.Internal,
					"failed to update view")
			}
		}

		// Broadcast changes.
		newPI.Do(func(p *skyobject.Pack, h *pack.Headers) error {
			changes := h.GetChanges()
			for {
				select {
				case bi.changesChan <- changes:
					return nil
				default:
					// Empty if too full.
					<-bi.changesChan
				}
			}
		})

		// Set new pack instance.
		return newPI, nil
	})
}

// ChangesChan for WebSocket goodness.
func (bi *BoardInstance) ChangesChan() chan *object.Changes {
	return bi.changesChan
}

func (bi *BoardInstance) Get(viewID, cmdID string, a ...interface{}) (interface{}, error) {
	bi.piMux.Lock()
	defer bi.piMux.Unlock()

	v, ok := bi.views[viewID]
	if !ok {
		return nil, boo.Newf(boo.NotFound,
			"view of id '%s' not found", viewID)
	}
	return v.Get(cmdID, a)
}

/*
	<<< Update? >>>
	>>> Whether a call to (*BoardInstance).Update() is needed.
*/

func (bi *BoardInstance) UpdateNeeded() bool {
	bi.needUpdateMux.RLock()
	defer bi.needUpdateMux.RUnlock()
	return bi.needUpdate
}

func (bi *BoardInstance) SetUpdateNeeded() {
	bi.needUpdateMux.Lock()
	defer bi.needUpdateMux.Unlock()
	bi.needUpdate = true
}

func (bi *BoardInstance) ClearUpdateNeeded() {
	bi.needUpdateMux.Lock()
	defer bi.needUpdateMux.Unlock()
	bi.needUpdate = false
}

/*
	<<< PACK INSTANCE >>>
*/

func (bi *BoardInstance) ChangePack(change func(oldPI *PackInstance) (*PackInstance, error)) error {
	bi.piMux.Lock()
	defer bi.piMux.Unlock()
	if pi, e := change(bi.pi); e != nil {
		return e
	} else {
		bi.pi = pi
		return nil
	}
}

func (bi *BoardInstance) PackRead(action PackAction) error {
	bi.piMux.Lock()
	defer bi.piMux.Unlock()
	return bi.pi.Do(func(p *skyobject.Pack, h *pack.Headers) error {
		return action(p, h)
	})
}

func (bi *BoardInstance) PackEdit(action PackAction) error {
	bi.piMux.Lock()
	defer bi.piMux.Unlock()
	e := bi.pi.Do(func(p *skyobject.Pack, h *pack.Headers) error {
		return action(p, h)
	})
	if e != nil {
		return e
	}
	bi.SetUpdateNeeded()
	return nil
}

/*
	<<< SEQUENCE >>>
*/

func (bi *BoardInstance) GetSeq() uint64 {
	var seq uint64
	bi.PackRead(func(p *skyobject.Pack, h *pack.Headers) error {
		seq = p.Root().Seq
		return nil
	})
	return seq
}

func (bi *BoardInstance) WaitSeq(ctx context.Context, goal uint64) error {
	if bi.GetSeq() >= goal {
		return nil
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if bi.GetSeq() >= goal {
				return nil
			}
		}
	}
}
