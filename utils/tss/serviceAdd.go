package tss

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"

	"github.com/getamis/alice/crypto/tss/ecdsa/addshare"
	"github.com/getamis/alice/crypto/tss/ecdsa/addshare/newpeer"
	"github.com/getamis/alice/crypto/tss/ecdsa/addshare/oldpeer"
	"github.com/getamis/alice/types"
	"google.golang.org/protobuf/proto"
)

//////////
/// Existing actors

type serviceAddExisting struct {
	pm          *PeerManager
	adder       *oldpeer.AddShare
	pubkey      *Pubkey
	share       string
	threshold   uint32
	newClientID string
	BKs         map[string]BK
	done        chan struct{}
	result      *oldpeer.Result
	err         error
}

func NewServiceAddExisting(pubkey *Pubkey, share string, threshold uint32, newClientID string, BKs map[string]BK) *serviceAddExisting {
	s := &serviceAddExisting{
		// pm:   pm,
		pubkey:      pubkey,
		share:       share,
		threshold:   threshold,
		newClientID: newClientID,
		BKs:         BKs,
		done:        make(chan struct{}),
	}

	return s
}

func (p *serviceAddExisting) GetDoneChan() chan struct{} {
	return p.done
}

func (p *serviceAddExisting) Init(pm *PeerManager) error {
	p.pm = pm

	// AddShare needs results from DKG.
	dkgResult, err := ConvertDKGResult(p.pubkey, p.share, p.BKs)
	if err != nil {
		log.Println("Cannot get DKG result", "err", err)
		return err
	}

	log.Println("serviceAddExisting.init - dkgResult:", dkgResult)

	oldPeerAddShare, err := oldpeer.NewAddShare(pm, dkgResult.PublicKey, p.threshold, dkgResult.Share, dkgResult.Bks, p.newClientID, p)
	if err != nil {
		log.Println("Cannot create a new AddShare", "err", err)
		return err
	}
	p.adder = oldPeerAddShare

	return nil
}

func (p *serviceAddExisting) Handle(msg types.Message) error {
	// log.Printf("Handle msg: %v\n", string(msg))

	err := p.adder.AddMessage(msg.GetId(), msg)
	return err
}

func (p *serviceAddExisting) Process() {
	// 1. Start a Signing process.
	p.adder.Start()
	defer p.adder.Stop()

	// 2. Wait the signing is done or failed
	<-p.done
}

func (p *serviceAddExisting) OnStateChanged(oldState types.MainState, newState types.MainState) {

	log.Println("serviceAddExisting - State changed", "old", oldState.String(), "new", newState.String())

	if newState == types.StateFailed {
		log.Println("Signing failed", "old", oldState.String(), "new", newState.String())
		p.err = fmt.Errorf("signing failed")
		close(p.done)
		return
	} else if newState == types.StateDone {
		// ATTENTION : concurrency problem => once either client or server has finished, he will close the connexion which might kill the last necessary message for the other one
		// => for now, implemented 1sec delay before closing so that everything can finish correctly

		// log.Println("Signing done", "old", oldState.String(), "new", newState.String())
		result, err := p.adder.GetResult()
		if err == nil {
			p.result = result
		} else {
			log.Println("Failed to get result from Signing", "err", err)
			p.err = err
		}
		close(p.done)
		return
	}
}

//////////
/// New actor

type serviceAddNew struct {
	pm        *PeerManager
	adder     *newpeer.AddShare
	pubkey    *Pubkey
	threshold uint32
	rank      uint32
	BKs       map[string]BK
	done      chan struct{}
	result    *newpeer.Result
	err       error
}

func NewServiceAddNew(pubkey *Pubkey, threshold, rank uint32, BKs map[string]BK) *serviceAddNew {
	s := &serviceAddNew{
		// pm:   pm,
		pubkey:    pubkey,
		threshold: threshold,
		rank:      rank,
		BKs:       BKs,
		done:      make(chan struct{}),
	}

	return s
}

func (p *serviceAddNew) GetDoneChan() chan struct{} {
	return p.done
}

func (p *serviceAddNew) Init(pm *PeerManager) error {
	p.pm = pm

	pubkey, err := p.pubkey.GetECPoint()
	if err != nil {
		log.Println("Cannot get pubkey", "err", err)
		return err
	}

	newPeerAddShare := newpeer.NewAddShare(pm, pubkey, p.threshold, p.rank, p)
	p.adder = newPeerAddShare

	return nil
}

func (p *serviceAddNew) Handle(msg types.Message) error {
	// log.Printf("Handle msg: %v\n", string(msg))

	err := p.adder.AddMessage(msg.GetId(), msg)
	return err
}

func (p *serviceAddNew) Process() {
	// 1. Start a Signing process.
	p.adder.Start()
	defer p.adder.Stop()

	// 2. Wait the signing is done or failed
	<-p.done
}

func (p *serviceAddNew) OnStateChanged(oldState types.MainState, newState types.MainState) {

	log.Println("serviceAddNew - State changed", "old", oldState.String(), "new", newState.String())

	if newState == types.StateFailed {
		log.Println("Signing failed", "old", oldState.String(), "new", newState.String())
		p.err = fmt.Errorf("signing failed")
		close(p.done)
		return
	} else if newState == types.StateDone {
		// ATTENTION : concurrency problem => once either client or server has finished, he will close the connexion which might kill the last necessary message for the other one
		// => for now, implemented 1sec delay before closing so that everything can finish correctly

		// log.Println("Signing done", "old", oldState.String(), "new", newState.String())
		result, err := p.adder.GetResult()
		if err == nil {
			p.result = result
		} else {
			log.Println("Failed to get result from Signing", "err", err)
			p.err = err
		}
		close(p.done)
		return
	}
}

///////////
/// TSS MANAGER ///
///////////

// ///////
// Server
type ServerAdd struct {
	service *serviceAddExisting
}

func NewServerAdd(pubkeyStr PubkeyStr, share string, BKs map[string]BK) (*ServerAdd, error) {
	// will probably need a wrapper with JSON input

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	service := NewServiceAddExisting(pubkey, share, _threshold, AddNewClientID, BKs)

	pm := NewPeerManager(_serverID)
	pm.AddPeer(AddExistingClientID)
	// pm.AddPeer(AddNewClientID)

	err = service.Init(pm)
	if err != nil {
		log.Println("error initialising service signer:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		// log.Println("handle message server")
		return service.Handle(msg)
	})

	return &ServerAdd{service: service}, nil
}

func (p *ServerAdd) GetDoneChan() chan struct{} {
	return p.service.GetDoneChan()
}

func (p *ServerAdd) Process() error { // Update to have result that makes sense
	p.service.Process()

	if p.service.result == nil {
		return fmt.Errorf("could not get server signer results")
	}

	log.Println("ServerAdd result:", p.service.result)
	log.Println("ServerAdd result share:", p.service.result.Share)

	return nil
}

func (p *ServerAdd) GetNextMessageToSend(peerID string) (Message, error) {
	return p.service.pm.GetNextMessageToSendPeer(peerID)
}

// Handle messages coming from clients : if the target is the server, consume; else, add to list of messages to be sent through MustSend
func (p *ServerAdd) HandleMessage(msg *Message) error {
	// log.Println("HandleMessage server")

	msgStr, ok := msg.Message.(string)
	if !ok {
		log.Println("msg was not a string")
		return errors.New("msg was not a string")
	}
	byteString, err := hex.DecodeString(msgStr)
	if err != nil {
		log.Println("error decoding hex:", err)
		return err
	}
	addMsg := &addshare.Message{}
	err = proto.Unmarshal(byteString, addMsg)
	// err = json.Unmarshal(byteString, addMsg)
	if err != nil {
		log.Println("ServerAdd.HandleMessage: could not proto unmarshal tss message to addshare.Message")
		return err
	}

	if msg.PeerID == _serverID { // server is the target peer
		return p.service.pm.HandleMessage(addMsg)
	} else {
		p.service.pm.MustSend(msg.PeerID, addMsg)
		return nil
	}
}

// ///////
// Existing client
type ExistingClientAdd struct {
	service *serviceAddExisting
}

func NewExistingClientAdd(pubkeyStr PubkeyStr, share string, BKs map[string]BK) (*ExistingClientAdd, error) {
	// will probably need a wrapper with JSON input

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	service := NewServiceAddExisting(pubkey, share, _threshold, AddNewClientID, BKs)

	pm := NewPeerManager(AddExistingClientID)
	pm.AddPeer(_serverID)
	// pm.AddPeer(AddNewClientID)

	err = service.Init(pm)
	if err != nil {
		log.Println("error initialising service signer:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		// log.Println("handle message server")
		return service.Handle(msg)
	})

	return &ExistingClientAdd{service: service}, nil
}

func (p *ExistingClientAdd) GetDoneChan() chan struct{} {
	return p.service.GetDoneChan()
}

func (p *ExistingClientAdd) Process() error { // Update to have result that makes sense
	p.service.Process()

	if p.service.result == nil {
		return fmt.Errorf("could not get server signer results")
	}

	log.Println("ExistingClientAdd result:", p.service.result)

	return nil
}

func (p *ExistingClientAdd) GetNextMessageToSend(peerID string) ([]byte, error) {
	return p.service.pm.GetNextMessageToSend(peerID)
}

func (p *ExistingClientAdd) GetNextMessageToSendAll() (Message, error) {
	return p.service.pm.GetNextMessageToSendAll()
}

func (p *ExistingClientAdd) HandleMessage(msg *Message) error {
	// log.Println("HandleMessage server")

	msgStr, ok := msg.Message.(string)
	if !ok {
		log.Println("msg was not a string")
		return errors.New("msg was not a string")
	}
	byteString, err := hex.DecodeString(msgStr)
	if err != nil {
		log.Println("error decoding hex:", err)
		return err
	}
	addMsg := &addshare.Message{}
	err = proto.Unmarshal(byteString, addMsg)
	// err = json.Unmarshal(byteString, addMsg)
	if err != nil {
		log.Println("ExistingClientAdd.HandleMessage: could not proto unmarshal tss message to addshare.Message")
		return err
	}

	if msg.PeerID == AddExistingClientID { // existing client is the target peer
		return p.service.pm.HandleMessage(addMsg)
	} else {
		p.service.pm.MustSend(msg.PeerID, addMsg)
		return nil
	}
}

// ///////
// New Client
type ClientAdd struct {
	service *serviceAddNew
}

func NewClientAdd(pubkeyStr PubkeyStr, BKs map[string]BK) (*ClientAdd, error) {
	// will probably need a wrapper with JSON input

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	service := NewServiceAddNew(pubkey, _threshold, _rank, BKs)

	pm := NewPeerManager(AddNewClientID)
	pm.AddPeer(_serverID)
	pm.AddPeer(AddExistingClientID)

	err = service.Init(pm)
	if err != nil {
		log.Println("error initialising service DKG:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		// log.Println("handle message client")
		return service.Handle(msg)
	})

	return &ClientAdd{service: service}, nil
}

func (p *ClientAdd) GetDoneChan() chan struct{} {
	return p.service.GetDoneChan()
}

func (p *ClientAdd) Process() (*DkgResult, error) { // Update to have result that makes sense
	p.service.Process()

	if p.service.result == nil {
		return nil, errors.New("could not get client dkg results")
	}

	pubkey := Pubkey{
		X: p.service.result.PublicKey.GetX(),
		Y: p.service.result.PublicKey.GetY(),
	}
	share := p.service.result.Share.String()
	BKs := make(map[string]BK)

	addr := pubkey.GetAddress().Hex()
	pubkeyStr := pubkey.GetStr()

	// Build bks.
	for peerID, bk := range p.service.result.Bks {
		BKs[peerID] = BK{
			X:    bk.GetX().String(),
			Rank: bk.GetRank(),
		}
	}

	dkgResult := DkgResult{
		Pubkey:  pubkeyStr,
		BKs:     BKs,
		Share:   share,
		Address: addr,
	}

	return &dkgResult, nil
}

func (p *ClientAdd) GetNextMessageToSend(peerID string) ([]byte, error) {
	return p.service.pm.GetNextMessageToSend(peerID)
}

func (p *ClientAdd) GetNextMessageToSendAll() (Message, error) {
	return p.service.pm.GetNextMessageToSendAll()
}

func (p *ClientAdd) HandleMessage(msg *Message) error {
	// log.Println("HandleMessage server")

	msgStr, ok := msg.Message.(string)
	if !ok {
		log.Println("msg was not a string")
		return errors.New("msg was not a string")
	}
	byteString, err := hex.DecodeString(msgStr)
	if err != nil {
		log.Println("error decoding hex:", err)
		return err
	}
	addMsg := &addshare.Message{}
	err = proto.Unmarshal(byteString, addMsg)
	// err = json.Unmarshal(byteString, addMsg)
	if err != nil {
		log.Println("ClientAdd.HandleMessage: could not proto unmarshal tss message to addshare.Message")
		return err
	}

	if msg.PeerID == AddNewClientID { // new client is the target peer
		return p.service.pm.HandleMessage(addMsg)
	} else {
		p.service.pm.MustSend(msg.PeerID, addMsg)
		return nil
	}
}
