package ws

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type MessageType struct { // should be in utils/ws.go ? (with the rest below)
	MsgType  string `json:"msgType"`
	MsgStage uint32 `json:"msgStage"`
}

var (
	PeerIdBroadcastMessage        = MessageType{MsgType: "peer", MsgStage: 10}
	DeviceMessage                 = MessageType{MsgType: "device", MsgStage: 20}       // new device to server (=> respond with pubkey)
	PubkeyMessage                 = MessageType{MsgType: "pubkey", MsgStage: 20}       // server to new device (=> start TSS on new device)
	PubkeyAckMessage              = MessageType{MsgType: "pubkey-ack", MsgStage: 30}   // new device to server (=> start TSS msg management on server registerHandler)
	MetadataMessage               = MessageType{MsgType: "metadata", MsgStage: 20}     // old device to server (before TSS) ; server to new device (after TSS)
	MetadataAckMessage            = MessageType{MsgType: "metadata-ack", MsgStage: 30} // server to old device (=> start TSS)
	TssMessage                    = MessageType{MsgType: "tss", MsgStage: 40}
	TssDoneMessage                = MessageType{MsgType: "tss-done", MsgStage: 50}
	EverythingStoredClientMessage = MessageType{MsgType: "stored-client", MsgStage: 70}
	ExistingDeviceDoneMessage     = MessageType{MsgType: "existing-device-done", MsgStage: 80}
	NewDeviceDoneMessage          = MessageType{MsgType: "new-device-done", MsgStage: 80}
	ErrorMessage                  = MessageType{MsgType: "error", MsgStage: 0}
)

type Message struct {
	Type MessageType `json:"type"`
	Msg  string      `json:"payload"`
}

// TssSend loops through TSS messages to be sent through the websocket connection
// Note : this is the v2 of tss.Send() used for AddDevice and Dkg (for now, Sign still uses tss.Send())
func TssSend(getNextMessageToSend func() (tss.Message, error), serverDone chan struct{}, errs chan error, ctx context.Context, c *websocket.Conn, functionName string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-serverDone:
			return
		default:
			tssMsg, err := getNextMessageToSend()
			if err != nil {
				if strings.Contains(err.Error(), "no message to be sent") {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				log.Println(functionName, "- error getting next message:", err)
				errs <- err
				return
			}

			if len(tssMsg.PeerID) == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			log.Println(functionName, "- got next message to send :", tssMsg)

			// format message for communication
			jsonEncodedMsg, err := json.Marshal(tssMsg)
			if err != nil {
				log.Println("could not marshal tss msg:", err)
				errs <- err
				return
			}

			payload := hex.EncodeToString(jsonEncodedMsg)

			msg := Message{
				Type: TssMessage,
				Msg:  payload,
			}

			// log.Println("trying send, next encoded message to send:", encodedMsg)

			if tssMsg.Message != nil {
				// log.Println("trying to send message:", encodedMsg)
				err := wsjson.Write(ctx, c, msg)
				if err != nil {
					log.Println("error writing json through websocket:", err)
					errs <- err
					return
				}

				log.Println(functionName, "- sent TssMsg message:", msg)
			}

			time.Sleep(10 * time.Millisecond)
		}
	}
}

func ProcessErrors(errs chan error, ctx context.Context, c *websocket.Conn, functionName string) error {
	select {
	case processErr := <-errs:
		if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
			log.Println(functionName, "- websocket closed normally")
			return nil
		} else if ctx.Err() == context.Canceled {
			log.Println(functionName, "- websocket closed by context cancellation:", processErr)
			return nil
		} else {
			log.Println(functionName, "- error during websocket connection:", processErr)
			return processErr
		}
	default:
		log.Println(functionName, "- no error during TSS")
		return nil
	}
}
