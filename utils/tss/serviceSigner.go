package tss

import (
	"errors"
	"fmt"
	"log"
	"math/big"

	"github.com/getamis/alice/crypto/birkhoffinterpolation"
	"github.com/getamis/alice/crypto/homo/paillier"
	"github.com/getamis/alice/crypto/tss/dkg"
	"github.com/getamis/alice/crypto/tss/ecdsa/gg18/signer"
	"github.com/getamis/alice/types"
)

type serviceSigner struct {
	pm      *PeerManager
	signer  *signer.Signer
	pubkey  *Pubkey
	share   string
	BKs     map[string]BK
	message []byte
	done    chan struct{}
	result  *signer.Result
	err     error
}

func NewServiceSigner(pubkey *Pubkey, share string, BKs map[string]BK, message []byte) *serviceSigner {
	s := &serviceSigner{
		// pm:   pm,
		pubkey:  pubkey,
		share:   share,
		BKs:     BKs,
		message: message,
		done:    make(chan struct{}),
	}

	return s
}

var (
	// ErrConversion for big int conversion error
	ErrConversion = errors.New("conversion error")
)

// ConvertDKGResult converts DKG result from config.
func ConvertDKGResult(k *Pubkey, cfgShare string, cfgBKs map[string]BK) (*dkg.Result, error) {
	// Build public key.
	// x, ok := new(big.Int).SetString(cfgPubkey.X, 10)
	// if !ok {
	// 	log.Error("Cannot convert string to big int", "x", cfgPubkey.X)
	// 	return nil, ErrConversion
	// }
	// y, ok := new(big.Int).SetString(cfgPubkey.Y, 10)
	// if !ok {
	// 	log.Error("Cannot convert string to big int", "y", cfgPubkey.Y)
	// 	return nil, ErrConversion
	// }
	pubkey, err := k.GetECPoint()
	if err != nil {
		log.Println("Cannot get public key", "err", err)
		return nil, err
	}

	// Build share.
	share, ok := new(big.Int).SetString(cfgShare, 10)
	if !ok {
		log.Println("Cannot convert string to big int", "share", share)
		return nil, ErrConversion
	}

	dkgResult := &dkg.Result{
		PublicKey: pubkey,
		Share:     share,
		Bks:       make(map[string]*birkhoffinterpolation.BkParameter),
	}

	// Build bks.
	for peerID, bk := range cfgBKs {
		x, ok := new(big.Int).SetString(bk.X, 10)
		if !ok {
			log.Println("Cannot convert string to big int", "x", bk.X)
			return nil, ErrConversion
		}
		dkgResult.Bks[peerID] = birkhoffinterpolation.NewBkParameter(x, bk.Rank)
	}

	return dkgResult, nil
}

func (p *serviceSigner) Init(pm *PeerManager) error {
	p.pm = pm

	// Signer needs results from DKG.
	dkgResult, err := ConvertDKGResult(p.pubkey, p.share, p.BKs)
	if err != nil {
		log.Println("Cannot get DKG result", "err", err)
		return err
	}

	// For simplicity, we use Paillier algorithm in signer.
	paillier, err := paillier.NewPaillier(2048)
	if err != nil {
		log.Println("Cannot create a paillier function", "err", err)
		return err
	}

	// Create signer
	signer, err := signer.NewSigner(pm, dkgResult.PublicKey, paillier, dkgResult.Share, dkgResult.Bks, p.message, p)
	if err != nil {
		log.Println("Cannot create a new signer", "err", err)
		return err
	}
	p.signer = signer

	return nil
}

func (p *serviceSigner) Handle(msg types.Message) error {
	// log.Printf("Handle msg: %v\n", string(msg))

	err := p.signer.AddMessage(msg.GetId(), msg)
	return err
}

func (p *serviceSigner) Process() {
	// 1. Start a Signing process.
	p.signer.Start()
	defer p.signer.Stop()

	// 2. Wait the signing is done or failed
	<-p.done
}

func (service *serviceSigner) PostProcess() (*Signature, error) {
	publicKeyECDSA := service.pubkey.GetECDSA()

	newR, newS, err := secp256k1SignatureToLowS(publicKeyECDSA, service.result.R, service.result.S)
	if err != nil {
		log.Println("error SignatureToLowS:", err)
		return nil, err
	}

	signature, err := GenerateSignature(newR, newS, service.pubkey, service.message)
	if err != nil {
		log.Println("error generating signature:", err)
		return nil, err
	}

	sig := Signature{
		R:         newR,
		S:         newS,
		Signature: signature,
	}

	return &sig, nil
}

func (p *serviceSigner) OnStateChanged(oldState types.MainState, newState types.MainState) {

	// log.Println("serviceSigner - State changed", "old", oldState.String(), "new", newState.String())

	if newState == types.StateFailed {
		log.Println("Signing failed", "old", oldState.String(), "new", newState.String())
		p.err = fmt.Errorf("signing failed")
		close(p.done)
		return
	} else if newState == types.StateDone {
		result, err := p.signer.GetResult()
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
