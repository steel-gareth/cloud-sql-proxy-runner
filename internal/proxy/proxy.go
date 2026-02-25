package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

type Dialer interface {
	Dial(ctx context.Context, instance string) (net.Conn, error)
	Close() error
}

type Listener struct {
	Instance string
	Port     int
	listener net.Listener
	dialer   Dialer
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewListener(instance string, port int, dialer Dialer) *Listener {
	return &Listener{
		Instance: instance,
		Port:     port,
		dialer:   dialer,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	addr := fmt.Sprintf("localhost:%d", l.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	l.listener = ln
	l.ctx, l.cancel = context.WithCancel(ctx)

	l.wg.Add(1)
	go l.acceptLoop()

	return nil
}

func (l *Listener) acceptLoop() {
	defer l.wg.Done()
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("accept error on port %d: %v", l.Port, err)
				return
			}
		}
		l.wg.Add(1)
		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(clientConn net.Conn) {
	defer l.wg.Done()
	defer clientConn.Close()

	remoteConn, err := l.dialer.Dial(l.ctx, l.Instance)
	if err != nil {
		log.Printf("dial error for %s: %v", l.Instance, err)
		return
	}
	defer remoteConn.Close()

	// Bidirectional copy
	done := make(chan struct{})
	go func() {
		io.Copy(remoteConn, clientConn)
		close(done)
	}()
	io.Copy(clientConn, remoteConn)
	<-done
}

func (l *Listener) Close() error {
	if l.cancel != nil {
		l.cancel()
	}
	if l.listener != nil {
		l.listener.Close()
	}
	l.wg.Wait()
	return nil
}

func (l *Listener) Addr() net.Addr {
	if l.listener != nil {
		return l.listener.Addr()
	}
	return nil
}
