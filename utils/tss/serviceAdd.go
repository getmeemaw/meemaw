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

func AdderGenericHandle(msg *Message, pm *PeerManager, peerID string) error {
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

	if msg.PeerID == peerID { // existing client is the target peer
		return pm.HandleMessage(addMsg)
	} else {
		pm.MustSend(msg.PeerID, addMsg)
		return nil
	}
}

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
	log.Println("serviceAddExisting.init - pm:", pm)

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

	// log.Println("serviceAddExisting - State changed", "old", oldState.String(), "new", newState.String())

	if newState == types.StateFailed {
		log.Println("Adding failed", "old", oldState.String(), "new", newState.String())
		p.err = fmt.Errorf("adding failed")
		close(p.done)
		return
	} else if newState == types.StateDone {
		result, err := p.adder.GetResult()
		if err == nil {
			p.result = result
		} else {
			log.Println("Failed to get result from Adding", "err", err)
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

	// log.Println("serviceAddNew - State changed", "old", oldState.String(), "new", newState.String())

	if newState == types.StateFailed {
		log.Println("Adding failed", "old", oldState.String(), "new", newState.String())
		p.err = fmt.Errorf("adding failed")
		close(p.done)
		return
	} else if newState == types.StateDone {
		result, err := p.adder.GetResult()
		if err == nil {
			p.result = result
		} else {
			log.Println("Failed to get result from Adding", "err", err)
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
	service     *serviceAddExisting
	originalBKs map[string]BK
}

func (p *ServerAdd) GetOriginalWallet() *DkgResult {
	pubkey := Pubkey{
		X: p.service.pubkey.X,
		Y: p.service.pubkey.Y,
	}
	share := p.service.share

	addr := pubkey.GetAddress().Hex()
	pubkeyStr := pubkey.GetStr()

	dkgResult := DkgResult{
		Pubkey:  pubkeyStr,
		BKs:     p.originalBKs,
		Share:   share,
		Address: addr,
		PeerID:  _serverID,
	}

	return &dkgResult
}

func NewServerAdd(newClientPeerID string, existingClientPeerID string, pubkeyStr PubkeyStr, share string, BKs map[string]BK) (*ServerAdd, error) {
	// will probably need a wrapper with JSON input

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	newBKs := make(map[string]BK)
	for key, value := range BKs {
		if key == _serverID || key == existingClientPeerID {
			newBKs[key] = value
		}
	}

	service := NewServiceAddExisting(pubkey, share, _threshold, newClientPeerID, newBKs)

	pm := NewPeerManager(_serverID)
	pm.AddPeer(existingClientPeerID)
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

	return &ServerAdd{service: service, originalBKs: BKs}, nil
}

func (p *ServerAdd) GetDoneChan() chan struct{} {
	return p.service.GetDoneChan()
}

func (p *ServerAdd) Process() (*DkgResult, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, errors.New("could not get serverAdd results")
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
		PeerID:  _serverID,
	}

	return &dkgResult, nil
}

func (p *ServerAdd) GetNextMessageToSend(peerID string) (Message, error) {
	return p.service.pm.GetNextMessageToSendPeer(peerID)
}

// Handle messages coming from clients : if the target is the server, consume; else, add to list of messages to be sent through MustSend
func (p *ServerAdd) HandleMessage(msg *Message) error {
	return AdderGenericHandle(msg, p.service.pm, _serverID)
}

// ///////
// Existing client
type ExistingClientAdd struct {
	service *serviceAddExisting
	peerID  string
}

func NewExistingClientAdd(newClientPeerID string, peerID string, pubkeyStr PubkeyStr, share string, BKs map[string]BK) (*ExistingClientAdd, error) {
	// will probably need a wrapper with JSON input

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	newBKs := make(map[string]BK)
	for key, value := range BKs {
		if key == _serverID || key == peerID {
			newBKs[key] = value
		}
	}

	service := NewServiceAddExisting(pubkey, share, _threshold, newClientPeerID, newBKs)

	pm := NewPeerManager(peerID)
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

	return &ExistingClientAdd{service: service, peerID: peerID}, nil
}

func (p *ExistingClientAdd) GetDoneChan() chan struct{} {
	return p.service.GetDoneChan()
}

func (p *ExistingClientAdd) Process() (*DkgResult, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, errors.New("could not get ExistingClientAdd dkg results")
	}

	res := ProcessResult{
		PublicKey: p.service.result.PublicKey,
		Share:     p.service.result.Share,
		Bks:       p.service.result.Bks,
		PeerID:    p.peerID,
	}

	return PostProcessResult(res)
}

func (p *ExistingClientAdd) GetNextMessageToSend(peerID string) ([]byte, error) {
	return p.service.pm.GetNextMessageToSend(peerID)
}

func (p *ExistingClientAdd) GetNextMessageToSendAll() (Message, error) {
	return p.service.pm.GetNextMessageToSendAll()
}

func (p *ExistingClientAdd) HandleMessage(msg *Message) error {
	return AdderGenericHandle(msg, p.service.pm, p.peerID)
}

// ///////
// New Client
type ClientAdd struct {
	service *serviceAddNew
	peerID  string
}

func NewClientAdd(peerID string, acceptingDevicePeerID string, pubkeyStr PubkeyStr, BKs map[string]BK) (*ClientAdd, error) {
	// will probably need a wrapper with JSON input

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	service := NewServiceAddNew(pubkey, _threshold, _rank, BKs)

	pm := NewPeerManager(peerID)
	pm.AddPeer(_serverID)
	pm.AddPeer(acceptingDevicePeerID)

	err = service.Init(pm)
	if err != nil {
		log.Println("error initialising service DKG:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		// log.Println("handle message client")
		return service.Handle(msg)
	})

	return &ClientAdd{service: service, peerID: peerID}, nil
}

func (p *ClientAdd) GetDoneChan() chan struct{} {
	return p.service.GetDoneChan()
}

func (p *ClientAdd) Process() (*DkgResult, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, errors.New("could not get ClientAdd dkg results")
	}

	res := ProcessResult{
		PublicKey: p.service.result.PublicKey,
		Share:     p.service.result.Share,
		Bks:       p.service.result.Bks,
		PeerID:    p.peerID,
	}

	return PostProcessResult(res)
}

func (p *ClientAdd) GetNextMessageToSend(peerID string) ([]byte, error) {
	return p.service.pm.GetNextMessageToSend(peerID)
}

func (p *ClientAdd) GetNextMessageToSendAll() (Message, error) {
	return p.service.pm.GetNextMessageToSendAll()
}

func (p *ClientAdd) HandleMessage(msg *Message) error {
	return AdderGenericHandle(msg, p.service.pm, p.peerID)
}
