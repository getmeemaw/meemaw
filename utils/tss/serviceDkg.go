package tss

import (
	"log"

	elliptic "github.com/getamis/alice/crypto/elliptic"
	"github.com/getamis/alice/crypto/tss/dkg"
	"github.com/getamis/alice/types"
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

func (p *serviceDkg) Handle(msg types.Message) error {
	// log.Printf("Handle msg: %+v\n", msg)

	err := p.dkg.AddMessage(msg.GetId(), msg)
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

	// log.Println("serviceDkg - State changed", "old", oldState.String(), "new", newState.String())

	if newState == types.StateFailed {
		log.Println("Dkg failed", "old", oldState.String(), "new", newState.String())
		log.Println("closing done channel")
		close(p.done)
		return
	} else if newState == types.StateDone {
		result, err := p.dkg.GetResult()
		if err == nil {
			p.result = result
		} else {
			log.Println("Failed to get result from DKG", "err", err)
		}
		log.Println("closing done channel")
		close(p.done)
		return
	}
}

type DKGResult struct {
	Share  string
	Pubkey Pubkey
	BKs    map[string]BK
}
