package tss

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/getamis/alice/types"
	"github.com/getamis/sirius/log"
	"google.golang.org/protobuf/proto"
)

type Message struct {
	PeerID  string
	Message interface{}
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

	// fmt.Println("Must send message to", peerID, ":", message)

	p.outwardMessages = append(p.outwardMessages, Message{
		PeerID:  peerID,
		Message: message,
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
	p.mu.Lock()
	defer p.mu.Unlock()

	p.peers[peerID] = true
}

func (p *PeerManager) GetNextMessageToSendAll() (Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.outwardMessages) == 0 {
		return Message{}, errors.New("no message to be sent")
	}

	ret := p.outwardMessages[0]

	msg, ok := ret.Message.(proto.Message)
	if !ok {
		return Message{}, fmt.Errorf("invalid proto message for %s : %v", ret.PeerID, ret.Message)
	}

	bs, err := proto.Marshal(msg)
	// bs, err := json.Marshal(ret.Message)
	if err != nil {
		log.Warn("Cannot marshal message", "err", err)
		return Message{}, err
	}

	ret.Message = hex.EncodeToString(bs)

	p.outwardMessages = p.outwardMessages[1:]

	return ret, nil
}

func (p *PeerManager) GetNextMessageToSendPeer(peerID string) (Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var nextMsg Message
	var newOutwardMessages []Message

	for _, el := range p.outwardMessages {
		if el.PeerID == peerID && len(nextMsg.PeerID) == 0 {
			// fmt.Println("GetNextMessageToSendPeer (", peerID, "):", el.Message)
			nextMsg.PeerID = el.PeerID
			msg, ok := el.Message.(proto.Message)
			if !ok {
				return Message{}, fmt.Errorf("invalid proto message for %s : %+v", peerID, el.Message)
			}

			bs, err := proto.Marshal(msg)
			// bs, err := json.Marshal(el.Message)
			if err != nil {
				log.Warn("Cannot marshal message", "err", err)
				return Message{}, err
			}

			nextMsg.Message = hex.EncodeToString(bs)
		} else {
			newOutwardMessages = append(newOutwardMessages, el)
		}
	}

	p.outwardMessages = newOutwardMessages

	return nextMsg, nil
}

// Not appropriate for Adder
func (p *PeerManager) GetNextMessageToSend(peerID string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var nextMsg []byte
	var newOutwardMessages []Message

	for _, el := range p.outwardMessages {
		if el.PeerID == peerID && len(nextMsg) == 0 {
			msg, ok := el.Message.(proto.Message)
			if !ok {
				return nil, fmt.Errorf("invalid proto message")
			}

			bs, err := proto.Marshal(msg)
			if err != nil {
				log.Warn("Cannot marshal message", "err", err)
				return nil, err
			}

			nextMsg = bs
		} else {
			newOutwardMessages = append(newOutwardMessages, el)
		}
	}

	p.outwardMessages = newOutwardMessages

	return nextMsg, nil
}

func (p *PeerManager) RegisterHandleMessage(handleFunc func(types.Message) error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.handleMessageFunction = handleFunc
}

func (p *PeerManager) HandleMessage(msg types.Message) error {
	return p.handleMessageFunction(msg)
}

// func remove(slice []Message, s int) []Message {
// 	return append(slice[:s], slice[s+1:]...)
// }
