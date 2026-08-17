package ami

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Event is a parsed AMI event or response
type Event map[string]string

type Client struct {
	addr   string
	user   string
	pass   string
	connMu sync.RWMutex // guards conn across reconnects
	conn   net.Conn

	mu       sync.Mutex
	handlers []func(Event)

	pendingMu sync.Mutex
	pending   map[string]chan Event
}

func NewClient(addr, user, pass string) *Client {
	return &Client{
		addr: addr,
		user: user,
		pass: pass,
	}
}

// OnEvent registers a handler called for every incoming event
func (c *Client) OnEvent(fn func(Event)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, fn)
}

// Connect dials and authenticates once, then starts the read loop in the
// background. It returns a channel that is closed when that connection later
// drops, so ConnectWithRetry knows when to reconnect.
func (c *Client) Connect() (<-chan struct{}, error) {
	conn, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("ami dial: %w", err)
	}
	scanner := bufio.NewScanner(conn)

	// Read banner
	scanner.Scan()

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	// Login
	if err := c.send("Action: Login\r\nUsername: " + c.user + "\r\nSecret: " + c.pass + "\r\n\r\n"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ami login send: %w", err)
	}

	// Read login response
	resp := readMessage(scanner)
	if resp == nil || resp["Response"] != "Success" {
		conn.Close()
		return nil, fmt.Errorf("ami login failed: %v", resp["Message"])
	}

	log.Println("[AMI] connected and authenticated")
	disconnected := make(chan struct{})
	go c.readLoop(disconnected, scanner)
	return disconnected, nil
}

// ConnectWithRetry connects and reconnects for the life of the process: a
// failed dial retries after a delay, and — unlike before — a connection that
// drops *after* connecting successfully is also retried instead of giving up
// permanently.
func (c *Client) ConnectWithRetry() {
	for {
		disconnected, err := c.Connect()
		if err != nil {
			log.Printf("[AMI] connect error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}
		<-disconnected
		log.Println("[AMI] disconnected — reconnecting in 5s")
		time.Sleep(5 * time.Second)
	}
}

// Action sends an AMI action and returns immediately (fire-and-forget)
func (c *Client) Action(fields map[string]string) error {
	var sb strings.Builder
	for k, v := range fields {
		sb.WriteString(k + ": " + v + "\r\n")
	}
	sb.WriteString("\r\n")
	return c.send(sb.String())
}

func (c *Client) send(s string) error {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	_, err := fmt.Fprint(conn, s)
	return err
}

func (c *Client) readLoop(disconnected chan struct{}, scanner *bufio.Scanner) {
	for {
		evt := readMessage(scanner)
		if evt == nil {
			log.Println("[AMI] connection closed")
			close(disconnected)
			return
		}

		if aid := evt["ActionID"]; aid != "" {
			c.pendingMu.Lock()
			ch, ok := c.pending[aid]
			c.pendingMu.Unlock()
			if ok {
				select {
				case ch <- evt:
				default: // slow/abandoned reader — drop rather than block the read loop
				}
			}
		}

		c.mu.Lock()
		for _, h := range c.handlers {
			h(evt)
		}
		c.mu.Unlock()
	}
}

func readMessage(scanner *bufio.Scanner) Event {
	evt := Event{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(evt) > 0 {
				return evt
			}
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			evt[parts[0]] = parts[1]
		}
	}
	return nil
}

// DialplanReload reloads the Asterisk dialplan (picks up changes from realtime/ast_extensions).
func (c *Client) DialplanReload() {
	_ = c.Action(map[string]string{
		"Action":  "Command",
		"Command": "dialplan reload",
	})
	log.Println("[AMI] dialplan reload sent")
}

// PJSIPReload reloads res_pjsip (picks up changes from realtime/ast_ps_* tables,
// e.g. a provider trunk's endpoint/aor/auth/identify/registration).
func (c *Client) PJSIPReload() {
	_ = c.Action(map[string]string{
		"Action":  "Command",
		"Command": "pjsip reload",
	})
	log.Println("[AMI] pjsip reload sent")
}

// CoreShowChannels requests current active channels
func (c *Client) CoreShowChannels() {
	_ = c.Action(map[string]string{
		"Action":  "CoreShowChannels",
		"ActionID": "show_channels",
	})
}

// QueueStatus requests queue statistics
func (c *Client) QueueStatus() {
	_ = c.Action(map[string]string{
		"Action":   "QueueStatus",
		"ActionID": "queue_status",
	})
}

// HangupChannel terminates a channel
func (c *Client) HangupChannel(channel string) {
	_ = c.Action(map[string]string{
		"Action":  "Hangup",
		"Channel": channel,
		"Cause":   "16",
	})
}

// QueryEvents sends an action tagged with a fresh ActionID and collects every
// event/response Asterisk sends back carrying that same ActionID, until a
// terminal one arrives (a plain Response with no "EventList: start", or an
// Event whose name ends in "Complete" — the standard AMI list-action pattern),
// or timeout elapses. Unlike Action(), this blocks for a reply — used for
// on-demand status queries (e.g. PJSIPShowRegistrationsOutbound) rather than
// the fire-and-forget reload/control actions above.
func (c *Client) QueryEvents(fields map[string]string, timeout time.Duration) ([]Event, error) {
	actionID := fmt.Sprintf("q-%d", time.Now().UnixNano())
	req := make(map[string]string, len(fields)+1)
	for k, v := range fields {
		req[k] = v
	}
	req["ActionID"] = actionID

	ch := make(chan Event, 64)
	c.pendingMu.Lock()
	if c.pending == nil {
		c.pending = make(map[string]chan Event)
	}
	c.pending[actionID] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, actionID)
		c.pendingMu.Unlock()
	}()

	if err := c.Action(req); err != nil {
		return nil, err
	}

	var events []Event
	deadline := time.After(timeout)
	for {
		select {
		case evt := <-ch:
			events = append(events, evt)
			if isTerminalEvent(evt) {
				return events, nil
			}
		case <-deadline:
			return events, fmt.Errorf("ami query timeout")
		}
	}
}

// isTerminalEvent reports whether evt ends an AMI action's reply: either a
// bare Response (single-reply actions) or an EventList-style "...Complete"
// event (list actions, which first ack with "Response: Success, EventList:
// start", then stream item events, then this).
func isTerminalEvent(evt Event) bool {
	if _, ok := evt["Response"]; ok {
		return evt["EventList"] != "start"
	}
	return strings.HasSuffix(evt["Event"], "Complete")
}

// ProviderRegistrationStatuses queries live outbound-registration status for
// every provider trunk that has REGISTER enabled, keyed by the registration
// object's id (see asterisk.ProviderRegistrationID, e.g. "reg-provider-3").
// Values are Asterisk's own status strings: "Registered", "Unregistered",
// "Rejected", "Auth Rejected", "No response", etc.
func (c *Client) ProviderRegistrationStatuses() (map[string]string, error) {
	events, err := c.QueryEvents(map[string]string{
		"Action": "PJSIPShowRegistrationsOutbound",
	}, 5*time.Second)
	if err != nil {
		log.Printf("[AMI][debug] PJSIPShowRegistrationsOutbound query error: %v", err)
		return nil, err
	}
	log.Printf("[AMI][debug] PJSIPShowRegistrationsOutbound: %d event(s)", len(events))
	for i, e := range events {
		log.Printf("[AMI][debug] event[%d] = %#v", i, e)
	}
	result := map[string]string{}
	for _, e := range events {
		if e["Event"] != "OutboundRegistrationDetail" {
			continue
		}
		if id := e["ObjectName"]; id != "" {
			result[id] = e["Status"]
		}
	}
	return result, nil
}

// Redirect moves a live channel to a new dialplan location. Used to pull a
// caller back out of the post-Queue hold (see writeKCDialplan) and into a
// fresh Dial() at the reconnecting agent's own extension once they're back —
// from the browser's side this just looks like a normal new incoming call.
func (c *Client) Redirect(channel, context, exten, priority string) error {
	events, err := c.QueryEvents(map[string]string{
		"Action":   "Redirect",
		"Channel":  channel,
		"Context":  context,
		"Exten":    exten,
		"Priority": priority,
	}, 5*time.Second)
	if err != nil {
		return err
	}
	if len(events) > 0 && !strings.EqualFold(events[0]["Response"], "Success") {
		return fmt.Errorf("redirect failed: %s", events[0]["Message"])
	}
	return nil
}

// PauseQueueMember pauses an agent
func (c *Client) PauseQueueMember(iface string, paused bool) {
	p := "false"
	if paused {
		p = "true"
	}
	_ = c.Action(map[string]string{
		"Action":    "QueuePause",
		"Interface": iface,
		"Paused":    p,
	})
}
