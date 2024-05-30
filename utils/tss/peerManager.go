package tss

import (
	"fmt"
	"sync"

	"github.com/getamis/alice/types"
	"github.com/getamis/sirius/log"
	"google.golang.org/protobuf/proto"
)

type Message struct {
	peerID  string
	message interface{}
}

type PeerManager struct {
	id                    string
	peers                 map[string]bool
	handleMessageFunction func(types.Message) error
	outwardMessages       []Message
	mu                    sync.Mutex
}

func NewPeerManager(id string) *PeerManager {
	return &PeerManager{
		id:    id,
		peers: make(map[string]bool),
	}
}

func (p *PeerManager) NumPeers() uint32 {
	return uint32(len(p.peers))
}

func (p *PeerManager) SelfID() string {
	return p.id
}

func (p *PeerManager) PeerIDs() []string {
	ids := make([]string, len(p.peers))
	i := 0
	for id := range p.peers {
		ids[i] = id
		i++
	}
	return ids
}

func (p *PeerManager) MustSend(peerID string, message interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.outwardMessages = append(p.outwardMessages, Message{
		peerID:  peerID,
		message: message,
	})
}

// // EnsureAllConnected connects the host to specified peer and sends the message to it.
// func (p *peerManager) EnsureAllConnected() {
// 	var wg sync.WaitGroup

// 	for _, peerAddr := range p.peers {
// 		wg.Add(1)
// 		go connectToPeer(p.host, peerAddr, &wg)
// 	}
// 	wg.Wait()
// }

// AddPeers adds peers to peer list.
func (p *PeerManager) AddPeer(peerID string) {
	p.peers[peerID] = true
}

func (p *PeerManager) GetNextMessageToSend(peerID string) ([]byte, error) {
	// log.Printf("get next message to send, outwardMessages:%v\n", p.outwardMessages)

	var nextMsg []byte
	i := 0

	for _, el := range p.outwardMessages {
		if el.peerID == peerID && len(nextMsg) == 0 {
			msg, ok := el.message.(proto.Message)
			if !ok {
				return nil, fmt.Errorf("invalid proto message")
			}

			// log.Printf("next message to send: %+v\n", msg)

			bs, err := proto.Marshal(msg)
			if err != nil {
				log.Warn("Cannot marshal message", "err", err)
				return nil, err
			}

			nextMsg = bs
		} else {
			p.mu.Lock()
			p.outwardMessages[i] = el
			p.mu.Unlock()
			i++
		}
	}

	p.outwardMessages = p.outwardMessages[:i]

	// log.Println("post GetNextMessageToSend outwardMessages:", p.outwardMessages)

	return nextMsg, nil
}

func (p *PeerManager) RegisterHandleMessage(handleFunc func(types.Message) error) {
	p.handleMessageFunction = handleFunc
}

func (p *PeerManager) HandleMessage(msg types.Message) error {
	return p.handleMessageFunction(msg)
}

// func remove(slice []Message, s int) []Message {
// 	return append(slice[:s], slice[s+1:]...)
// }
