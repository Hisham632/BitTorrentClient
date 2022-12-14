package torrent

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"time"
)

type Client struct {
	Connection net.Conn
	isChoked   bool
	bitfield   bitfield
	peerID     [20]byte
	address    net.TCPAddr

	hasDHT  bool
	dhtPort int
}

type Handshake struct {
	Protocol string
	InfoHash [20]byte
	PeerID   [20]byte
}

func ConnetingToClient(infoHash, peerID [20]byte, address net.TCPAddr) (*Client, error) {

	connection, err := net.DialTimeout("tcp", address.String(), 8*time.Second)

	if err != nil {
		return nil, err
	}

	peerClient := Client{
		Connection: connection,
		address:    address,
		isChoked:   true,
	}

	peerClient.Connection.SetDeadline(time.Now().Add(time.Second * 8))

	defer peerClient.Connection.SetDeadline(time.Time{})

	resp, err := peerClient.handshake(infoHash, peerID)

	if err != nil {
		return nil, fmt.Errorf("%s handshake %s", resp, err)
	}

	if len(peerClient.bitfield) == 0 {

		peerClient.Connection.SetDeadline(time.Now().Add(time.Second * 8))
		_, err = peerClient.RecieiveMessage()

		if err != nil {
			return nil, fmt.Errorf("Receiving bitfield message")
		}
		if len(peerClient.bitfield) == 0 {
			return nil, fmt.Errorf("bitfield not set")
		}
	}

	if peerClient.hasDHT {

		peerClient.Connection.SetDeadline(time.Now().Add(time.Second * 8))

		for count := 0; count < 50 && peerClient.dhtPort == 0; count++ {
			peerClient.RecieiveMessage()
		}

	}

	peerClient.Connection.SetDeadline(time.Now().Add(time.Second * 8))

	err = peerClient.SendMessage(MsgUnchoke, nil)
	if err != nil {
		return nil, fmt.Errorf("Sending unchoke:")
	}

	err = peerClient.SendMessage(MsgInterested, nil)
	if err != nil {
		return nil, fmt.Errorf("Sending interested")
	}

	return &peerClient, nil

}

func (peerClient *Client) handshake(infoHash, peerID [20]byte) (*Handshake, error) {

	peerHandshake := Handshake{
		Protocol: "BitTorrent protocol",
		InfoHash: infoHash,
		PeerID:   peerID,
	}

	var buffer bytes.Buffer
	buffer.WriteByte(byte(len(peerHandshake.Protocol)))
	buffer.WriteString(peerHandshake.Protocol)

	dhtByte := make([]byte, 8)
	dhtByte[7] |= 1
	buffer.Write(dhtByte)

	buffer.Write(peerHandshake.InfoHash[:])
	buffer.Write(peerID[:])

	_, err := peerClient.Connection.Write(buffer.Bytes())

	if err != nil {
		return nil, fmt.Errorf("sending handshake message to %s: %s", peerClient.Connection.RemoteAddr(), err)
	}

	bufferLenght := make([]byte, 1)
	_, err = io.ReadFull(peerClient.Connection, bufferLenght)

	if err != nil {
		return nil, fmt.Errorf("Problem in reading handshake %s", err)
	}

	protocolLenght := int(bufferLenght[0])
	if protocolLenght != 19 {
		return nil, fmt.Errorf("protocolLenght is not equal to 19")
	}

	handshakeBuffer := make([]byte, protocolLenght+48)
	_, err = io.ReadFull(peerClient.Connection, handshakeBuffer)

	if err != nil {
		return nil, err
	}

	read := protocolLenght
	var respExtensions [8]byte
	read += copy(respExtensions[:], handshakeBuffer[read:read+8])

	if respExtensions[7]|1 != 0 {
		peerClient.hasDHT = true
	}

	var responceInfoHash [20]byte

	read += copy(responceInfoHash[:], handshakeBuffer[read:read+20])
	copy(peerClient.peerID[:], handshakeBuffer[read:])

	responceHandshake := Handshake{
		Protocol: string(handshakeBuffer[0:protocolLenght]),
		InfoHash: responceInfoHash,
		PeerID:   peerClient.peerID,
	}

	if !bytes.Equal(responceHandshake.InfoHash[:], infoHash[:]) {
		return nil, fmt.Errorf("InfoHash dont match")
	}

	return &responceHandshake, nil
}
