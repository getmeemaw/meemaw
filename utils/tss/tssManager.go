package tss

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/getamis/alice/crypto/ecpointgrouplaw"
	elliptic_alice "github.com/getamis/alice/crypto/elliptic"
	"github.com/getamis/alice/crypto/tss/dkg"
	"github.com/getamis/alice/crypto/tss/ecdsa/gg18/signer"
	"github.com/getamis/alice/types"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/decred/dcrd/dcrec/secp256k1"

	secp_ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

type GenericTSS interface {
	// Process() (*Signature, error)
	// ProcessStr() (string, error)
	GetNextMessageToSend() ([]byte, error)
	HandleMessage(types.Message) error
	DkgOrSign() int
}

type PubkeyStr struct {
	X string
	Y string
}

type Pubkey struct {
	X *big.Int
	Y *big.Int
}

type BK struct {
	X    string
	Rank uint32
}

type SigningParameters struct {
	Pubkey PubkeyStr
	BKs    map[string]BK
}

func NewPubkey(k PubkeyStr) (*Pubkey, error) {
	X, ok := new(big.Int).SetString(k.X, 10)
	if !ok {
		return nil, fmt.Errorf("invalid hex for X")
	}

	Y, ok := new(big.Int).SetString(k.Y, 10)
	if !ok {
		return nil, fmt.Errorf("invalid hex for Y")
	}

	return &Pubkey{X: X, Y: Y}, nil
}

func (k *Pubkey) GetECDSA() *ecdsa.PublicKey {
	return &ecdsa.PublicKey{Curve: elliptic_alice.Secp256k1(), X: k.X, Y: k.Y}
}

func (k *Pubkey) GetAddress() common.Address {
	publicKeyECDSA := k.GetECDSA()
	addr := PubkeyToAddress(*publicKeyECDSA)
	return addr
}

func (k *Pubkey) GetBytes() []byte {
	publicKeyECDSA := k.GetECDSA()
	bytes := FromECDSAPub(publicKeyECDSA)
	return bytes
}

func (k *Pubkey) GetStr() PubkeyStr {
	return PubkeyStr{
		X: k.X.String(),
		Y: k.Y.String(),
	}
}

func (k *Pubkey) GetECPoint() (*ecpointgrouplaw.ECPoint, error) {
	curve := elliptic_alice.Secp256k1()
	return ecpointgrouplaw.NewECPoint(curve, k.X, k.Y)
}

///////////
/// DKG ///
///////////

type DkgResult struct {
	Pubkey  PubkeyStr
	BKs     map[string]BK
	Share   string
	Address string
}

type ServerDkg struct {
	service *serviceDkg
}

// Server
func NewServerDkg() (*ServerDkg, error) {
	service := NewServiceDkg(2, 0, elliptic_alice.Secp256k1())

	pm := NewPeerManager("server")
	pm.AddPeer("client")

	err := service.Init(pm)
	if err != nil {
		log.Println("error initialising service DKG:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		return service.Handle(msg)
	})

	return &ServerDkg{service: service}, nil
}

func (p *ServerDkg) Process() (*DkgResult, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, fmt.Errorf("could not get server dkg results")
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

func (p *ServerDkg) GetNextMessageToSend() ([]byte, error) {
	return p.service.pm.GetNextMessageToSend("client")
}

func (p *ServerDkg) HandleMessage(msg types.Message) error {
	return p.service.pm.HandleMessage(msg)
}

func (p *ServerDkg) DkgOrSign() int {
	return 1
}

// Client
type ClientDkg struct {
	service *serviceDkg
}

// Server
func NewClientDkg() (*ClientDkg, error) {
	service := NewServiceDkg(2, 0, elliptic_alice.Secp256k1())

	pm := NewPeerManager("client")
	pm.AddPeer("server")

	err := service.Init(pm)
	if err != nil {
		log.Println("error initialising service DKG:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		return service.Handle(msg)
	})

	return &ClientDkg{service: service}, nil
}

func (p *ClientDkg) Process() (*DkgResult, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, fmt.Errorf("could not get client dkg results")
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

func (p *ClientDkg) GetNextMessageToSend() ([]byte, error) {
	return p.service.pm.GetNextMessageToSend("server")
}

func (p *ClientDkg) HandleMessage(msg types.Message) error {
	return p.service.pm.HandleMessage(msg)
}

func (p *ClientDkg) DkgOrSign() int {
	return 1
}

////////////
/// SIGN ///
////////////

type Signature struct {
	R         *big.Int
	S         *big.Int
	Signature []byte
}

// type SignatureStr struct {
// 	R string
// 	S string
// 	V string
// }

// Server
type ServerSigner struct {
	service *serviceSigner
}

func NewServerSigner(pubkeyStr PubkeyStr, share string, BKs map[string]BK, message []byte) (*ServerSigner, error) {
	// will probably need a wrapper with JSON input

	curve := elliptic_alice.Secp256k1()

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	service := NewServiceSigner(pubkey, share, BKs, message, curve)

	pm := NewPeerManager("server")
	pm.AddPeer("client")

	err = service.Init(pm)
	if err != nil {
		log.Println("error initialising service signer:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		// log.Println("handle message server")
		return service.Handle(msg)
	})

	return &ServerSigner{service: service}, nil
}

func (p *ServerSigner) Process() (*Signature, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, fmt.Errorf("could not get server signer results")
	}

	publicKeyECDSA := p.service.pubkey.GetECDSA()

	newR, newS, err := secp256k1SignatureToLowS(publicKeyECDSA, p.service.result.R, p.service.result.S)
	if err != nil {
		log.Println("error SignatureToLowS:", err)
		return nil, err
	}

	signature, err := GenerateSignature(newR, newS, p.service.pubkey, p.service.message)
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

func (p *ServerSigner) GetNextMessageToSend() ([]byte, error) {
	return p.service.pm.GetNextMessageToSend("client")
}

func (p *ServerSigner) HandleMessage(msg types.Message) error {
	// log.Println("HandleMessage server")
	return p.service.pm.HandleMessage(msg)
}

func (p *ServerSigner) DkgOrSign() int {
	return 2
}

// Client
type ClientSigner struct {
	service *serviceSigner
}

func NewClientSigner(pubkeyStr PubkeyStr, share string, BKs map[string]BK, message []byte) (*ClientSigner, error) {
	// will probably need a wrapper with JSON input

	curve := elliptic_alice.Secp256k1()

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	service := NewServiceSigner(pubkey, share, BKs, message, curve)

	pm := NewPeerManager("client")
	pm.AddPeer("server")

	err = service.Init(pm)
	if err != nil {
		log.Println("error initialising service DKG:", err)
		return nil, err
	}

	pm.RegisterHandleMessage(func(msg types.Message) error {
		// log.Println("handle message client")
		return service.Handle(msg)
	})

	return &ClientSigner{service: service}, nil
}

func (p *ClientSigner) Process() (*Signature, error) {
	p.service.Process()

	if p.service.result == nil {
		return nil, fmt.Errorf("could not get client signer results")
	}

	publicKeyECDSA := p.service.pubkey.GetECDSA()

	newR, newS, err := secp256k1SignatureToLowS(publicKeyECDSA, p.service.result.R, p.service.result.S)
	if err != nil {
		log.Println("error SignatureToLowS:", err)
		return nil, err
	}

	signature, err := GenerateSignature(newR, newS, p.service.pubkey, p.service.message)
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

// func (p *ClientSigner) ProcessString() (string, error) {
// 	sig, err := p.Process()

// 	sigStr := SignatureStr{
// 		R: sig.R.Text(16),
// 		S: sig.S.Text(16),
// 		V: hex.EncodeToString(sig.V),
// 	}

// 	res, err := json.Marshal(sigStr)
// 	if err != nil {
// 		return "", err
// 	}

// 	return string(res), nil
// }

func (p *ClientSigner) GetNextMessageToSend() ([]byte, error) {
	return p.service.pm.GetNextMessageToSend("server")
}

func (p *ClientSigner) HandleMessage(msg types.Message) error {
	// log.Println("HandleMessage client")
	return p.service.pm.HandleMessage(msg)
}

func (p *ClientSigner) Test() []byte {
	// log.Println("HandleMessage client")
	return p.service.message
}

func (p *ClientSigner) DkgOrSign() int {
	return 2
}

///////////////////////
/// UTILS WEBSOCKET ///
///////////////////////

func Receive(tss GenericTSS, ctx context.Context, errs chan error, c *websocket.Conn) {
	// defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			log.Println("done receiving because of <-ctx.Done()")
			return
		default:
			var v string
			err := wsjson.Read(ctx, c, &v)
			if err != nil {
				if ctx.Err() == context.Canceled {
					log.Println("read operation canceled")
					errs <- err
					return
				} else {
					log.Println("error reading message from websocket:", err)
					log.Println("websocket.CloseStatus(err):", websocket.CloseStatus(err))
					errs <- err
					return
				}
			}
			// log.Printf("Received: %v\n", v)

			tssType := tss.DkgOrSign()
			msg, err := stringToMsg(v, tssType)
			if err != nil {
				log.Println("could not convert string to message:", err)
				errs <- err
				return
			}

			err = tss.HandleMessage(msg)
			if err != nil {
				log.Println("could not handle tss msg:", err)
				errs <- err
				return
			}
		}
	}
}

func Send(tss GenericTSS, ctx context.Context, errs chan error, c *websocket.Conn) {
	// defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			log.Println("done sending because of <-ctx.Done()")
			return
		default:
			msg, err := tss.GetNextMessageToSend()
			if err != nil {
				log.Println("error getting next message:", err)
				errs <- err
				return
			}

			encodedMsg := hex.EncodeToString(msg)

			// log.Println("trying send, next encoded message to send:", encodedMsg)

			if msg != nil {
				// log.Println("trying to send message:", encodedMsg)
				err := wsjson.Write(ctx, c, encodedMsg)
				if err != nil {
					log.Println("error writing json through websocket:", err)
					errs <- err
					return
				}
			}

			time.Sleep(10 * time.Millisecond) // UPDATE : remove polling, use channels to trigger send when next TSS message ready
		}
	}
}

func stringToMsg(input string, tssType int) (types.Message, error) {
	byteString, err := hex.DecodeString(input)
	if err != nil {
		log.Println("error decoding hex:", err)
		return nil, err
	}

	var data types.Message

	if tssType == 1 { // dkg
		dkgData := &dkg.Message{}
		err = proto.Unmarshal(byteString, dkgData)
		if err == nil {
			data = dkgData
		}
	} else if tssType == 2 { // sign
		signData := &signer.Message{}
		err = proto.Unmarshal(byteString, signData)
		if err == nil {
			data = signData
		}
	} else {
		log.Println("unrecognized tss type")
		return nil, errors.New("unrecognized tss type")
	}

	if err != nil {
		log.Println("Cannot unmarshal data", "err", err)
		return nil, err
	}

	return data, nil
}

///////////////////////
/// UTILS SIGNATURE ///
///////////////////////

// Mix of following links :
// https://github.com/golang/go/issues/54549
// https://cryptobook.nakov.com/digital-signatures/ecdsa-sign-verify-messages
// https://github.com/ethereum/go-ethereum/blob/e2507a17e8df5bb84b4b1195cf6a2d58e3ba109c/crypto/secp256k1/libsecp256k1/src/secp256k1.c#L272
// https://github.com/copernet/secp256k1-go/blob/master/secp256k1/secp256k1.go
// https://github.com/cryptocoinjs/secp256k1-node/blob/master/lib/elliptic_alice.js#L193
// https://stackoverflow.com/questions/74338846/ecdsa-signature-verification-mismatch

func secp256k1SignatureToLowS(k *ecdsa.PublicKey, r, s *big.Int) (*big.Int, *big.Int, error) {
	s, err := ToLowS(k, s)
	if err != nil {
		return nil, nil, err
	}

	return r, s, nil
}

// IsLow checks that s is a low-S
func IsLowS(k *ecdsa.PublicKey, s *big.Int) (bool, error) {
	N := k.Params().N
	halfOrder := new(big.Int).Rsh(N, 1)
	return s.Cmp(halfOrder) != 1, nil
}

func ToLowS(k *ecdsa.PublicKey, s *big.Int) (*big.Int, error) {
	lowS, err := IsLowS(k, s)
	if err != nil {
		return nil, err
	}

	if !lowS {
		// Set s to N - s that will be then in the lower part of signature space
		// less or equal to half order
		s.Sub(k.Params().N, s)
		return s, nil
	}

	return s, nil
}

// PubKeyAddr returns the Ethereum address for (uncompressed-)key bytes.
func pubKeyAddr(bytes []byte) common.Address {
	digest := Keccak256(bytes[1:])
	var addr common.Address
	copy(addr[:], digest[12:])
	return addr
}

func FromECDSAPub(pub *ecdsa.PublicKey) []byte {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil
	}
	return elliptic.Marshal(S256(), pub.X, pub.Y)
}

func PubkeyToAddress(p ecdsa.PublicKey) common.Address {
	pubBytes := FromECDSAPub(&p)
	return common.BytesToAddress(Keccak256(pubBytes[1:])[12:])
}

// Keccak256 calculates and returns the Keccak256 hash of the input data.
func Keccak256(data ...[]byte) []byte {
	b := make([]byte, 32)
	d := NewKeccakState()
	for _, b := range data {
		d.Write(b)
	}
	d.Read(b)
	return b
}

// NewKeccakState creates a new KeccakState
func NewKeccakState() KeccakState {
	return sha3.NewLegacyKeccak256().(KeccakState)
}

// because it doesn't copy the internal state, but also modifies the internal state.
type KeccakState interface {
	hash.Hash
	Read([]byte) (int, error)
}

// S256 returns an instance of the secp256k1 curve.
func S256() elliptic.Curve {
	return secp256k1.S256()
}

func GenerateSignature(r, s *big.Int, pubkey *Pubkey, msg []byte) ([]byte, error) {
	// https://github.com/pascaldekloe/etherkeyms/blob/main/etherkeyms.go
	var rLen, sLen int // byte size
	if r != nil {
		rLen = (r.BitLen() + 7) / 8
	}
	if s != nil {
		sLen = (s.BitLen() + 7) / 8
	}
	if rLen == 0 || rLen > 32 || sLen == 0 || sLen > 32 {
		return nil, fmt.Errorf("google KMS asymmetric signature with %d-byte r and %d-byte s denied on size", rLen, sLen)
	}

	// Need uncompressed signature with "recovery ID" at end:
	// https://bitcointalk.org/index.php?topic=5249677.0
	// https://ethereum.stackexchange.com/a/53182/39582
	var sig [66]byte // + 1-byte header + 1-byte tailer
	r.FillBytes(sig[33-rLen : 33])
	s.FillBytes(sig[65-sLen : 65])

	// brute force try includes KMS verification
	var recoverErr error
	for recoveryID := byte(0); recoveryID < 2; recoveryID++ {
		// log.Println("recoveryID:", recoveryID)
		sig[0] = recoveryID + 27 // BitCoin header
		btcsig := sig[:65]       // exclude Ethereum 'v' parameter
		// pubKey, _, err := btcecdsa.RecoverCompact(btcsig, msg[:])
		pubKey, _, err := secp_ecdsa.RecoverCompact(btcsig, msg)
		if err != nil {
			recoverErr = err
			continue
		}

		if pubKeyAddr(pubKey.SerializeUncompressed()) == pubkey.GetAddress() {
			// log.Println("pubkeys match")
			// sign the transaction
			sig[65] = recoveryID // Ethereum 'v' parameter
			break
			// exclude BitCoin header
		}
	}

	if recoverErr != nil {
		log.Println("error identifying recoveryID:", recoverErr)
		return nil, recoverErr
	}

	newSignature := sig[1:]

	// log.Println("recoverErr:", recoverErr)
	// log.Println("newSignature:", newSignature)

	// log.Println("R length:", len(r.Bytes()))
	// log.Println("S length:", len(s.Bytes()))

	// log.Println("NewSignature length:", len(newSignature))
	// log.Println("publicKeyBytes length:", len(pubkey.GetBytes()))
	// log.Println("messageBytes length:", len(msg))

	ok := ecdsa.Verify(pubkey.GetECDSA(), msg, r, s)
	// log.Println("verified ECDSA newSignature?", ok)

	if !ok {
		return nil, fmt.Errorf("signature does not ECDSA verify")
	}

	// ok = crypto.VerifySignature(pubkey.GetBytes(), msg, newSignature[:64])
	// log.Println("verified go-ethereum newSignature?", ok)

	return newSignature, nil
}

///////////////////////////
/// RECOVER PRIVATE KEY ///
///////////////////////////

func RecoverPrivateKeyWrapper(pubkeyStr PubkeyStr, serverShareStr string, clientShareStr string, BKs map[string]BK) (*ecdsa.PrivateKey, error) {
	curve := elliptic_alice.Secp256k1()

	pubkey, err := NewPubkey(pubkeyStr)
	if err != nil {
		return nil, err
	}

	ECPoint, err := pubkey.GetECPoint()
	if err != nil {
		return nil, err
	}

	dkgResultServer, err := ConvertDKGResult(pubkey, serverShareStr, BKs, curve)
	if err != nil {
		return nil, err
	}

	clientShare, ok := new(big.Int).SetString(clientShareStr, 10)
	if !ok {
		log.Println("Cannot convert string to big int", "share", clientShareStr)
		return nil, ErrConversion
	}

	RecoveryPeers := make([]RecoveryPeer, 0)

	RecoveryPeers = append(RecoveryPeers, RecoveryPeer{
		share: dkgResultServer.Share,
		bk:    dkgResultServer.Bks["server"],
	})

	RecoveryPeers = append(RecoveryPeers, RecoveryPeer{
		share: clientShare,
		bk:    dkgResultServer.Bks["client"],
	})

	return RecoverPrivateKey(curve, 2, ECPoint, RecoveryPeers)
}
