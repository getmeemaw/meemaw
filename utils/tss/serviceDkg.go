package tss

import (
	"log"

	elliptic "github.com/getamis/alice/crypto/elliptic"
	"github.com/getamis/alice/crypto/tss/dkg"
	"github.com/getamis/alice/types"
	"google.golang.org/protobuf/proto"
)

type serviceDkg struct {
	pm        *PeerManager
	dkg       *dkg.DKG
	threshold uint32
	rank      uint32
	curve     elliptic.Curve
	done      chan struct{}
	result    *dkg.Result
}

func NewServiceDkg(threshold, rank uint32, curve elliptic.Curve) *serviceDkg {
	s := &serviceDkg{
		// pm:   pm,
		threshold: threshold,
		rank:      rank,
		curve:     curve,
		done:      make(chan struct{}),
	}

	return s
}

func (p *serviceDkg) Init(pm *PeerManager) error {
	p.pm = pm

	// Create dkg
	d, err := dkg.NewDKG(p.curve, pm, p.threshold, p.rank, p)
	if err != nil {
		log.Println("Cannot create a new DKG", "err", err)
		return err
	}
	p.dkg = d

	return nil
}

func (p *serviceDkg) Handle(msg []byte) error {
	// log.Printf("Handle msg: %v\n", string(msg))

	data := &dkg.Message{}
	err := proto.Unmarshal(msg, data)
	if err != nil {
		log.Println("Cannot unmarshal data", "err", err)
		return err
	}

	err = p.dkg.AddMessage(data.GetId(), data)

	return err
}

func (p *serviceDkg) Process() {
	// 1. Start a DKG process.
	p.dkg.Start()
	defer p.dkg.Stop()

	// 2. Wait the dkg is done or failed
	<-p.done
}

func (p *serviceDkg) OnStateChanged(oldState types.MainState, newState types.MainState) {
	if newState == types.StateFailed {
		log.Println("Dkg failed", "old", oldState.String(), "new", newState.String())
		close(p.done)
		return
	} else if newState == types.StateDone {
		// ATTENTION : concurrency problem => once either client or server has finished, he will close the connexion which might kill the last necessary message for the other one
		// => for now, implemented 1sec delay before closing so that everything can finish correctly

		// log.Println("Dkg done", "old", oldState.String(), "new", newState.String())
		result, err := p.dkg.GetResult()
		if err == nil {
			p.result = result
		} else {
			log.Println("Failed to get result from DKG", "err", err)
		}
		close(p.done)
		return
	}

	// log.Println("State changed", "old", oldState.String(), "new", newState.String())
}

type DKGResult struct {
	Share  string
	Pubkey Pubkey
	BKs    map[string]BK
}
