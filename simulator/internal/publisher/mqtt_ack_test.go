package publisher

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"smt-digital-twin/simulator/internal/contract"
)

func readPacket(r *bufio.Reader) (byte, []byte, error) {
	header, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	size, mult := 0, 1
	for i := 0; i < 4; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		size += int(b&127) * mult
		if b&128 == 0 {
			payload := make([]byte, size)
			_, err = io.ReadFull(r, payload)
			return header, payload, err
		}
		mult *= 128
	}
	return 0, nil, errors.New("invalid MQTT length")
}

func TestMQTTDisconnectBeforePUBACKRetriesIdentity(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	type delivery struct {
		topic   string
		payload string
	}
	results := make(chan []delivery, 1)
	failures := make(chan error, 1)
	go func() {
		var deliveries []delivery
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				failures <- err
				return
			}
			conn.SetDeadline(time.Now().Add(10 * time.Second))
			reader := bufio.NewReader(conn)
			header, _, err := readPacket(reader)
			if err != nil || header>>4 != 1 {
				failures <- fmt.Errorf("CONNECT: %x %v", header, err)
				conn.Close()
				return
			}
			if _, err = conn.Write([]byte{0x20, 0x02, 0x00, 0x00}); err != nil {
				failures <- err
				conn.Close()
				return
			}
			header, body, err := readPacket(reader)
			if err != nil || header>>4 != 3 || header&6 != 2 || header&1 != 0 || len(body) < 4 {
				failures <- fmt.Errorf("QoS1 PUBLISH: %x %v", header, err)
				conn.Close()
				return
			}
			n := int(binary.BigEndian.Uint16(body[:2]))
			if len(body) < 2+n+2 {
				failures <- errors.New("short PUBLISH")
				conn.Close()
				return
			}
			topic := string(body[2 : 2+n])
			packetID := body[2+n : 4+n]
			deliveries = append(deliveries, delivery{topic, string(body[4+n:])})
			if i == 1 {
				if _, err = conn.Write([]byte{0x40, 0x02, packetID[0], packetID[1]}); err != nil {
					failures <- err
					conn.Close()
					return
				}
			}
			conn.Close()
		}
		results <- deliveries
	}()
	sink, err := NewMQTT(Config{URL: "tcp://" + listener.Addr().String(), ClientID: "ack-test", RetryMin: 10 * time.Millisecond, RetryMax: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "event-1", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, Payload: map[string]any{"synthetic": true, "workOrderId": "wo-1", "quantity": 1}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sink.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-results:
		if len(got) != 2 || got[0] != got[1] {
			t.Fatalf("retry changed event: %+v", got)
		}
	case err := <-failures:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := sink.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
