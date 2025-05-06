//go:build darwin

package fdnotifier

import (
	"fmt"

	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/sync"
	"gvisor.dev/gvisor/pkg/waiter"
)

type fdInfo struct {
	queue   *waiter.Queue
	waiting bool
}

// notifier holds all the state necessary to issue notifications when IO events
// occur in the observed FDs.
type notifier struct {
	// kqFD is the kqueue file descriptor used to register for io
	// notifications.
	kqFD int

	// mu protects fdMap.
	mu sync.Mutex

	// fdMap maps file descriptors to their notification queues and waiting
	// status.
	fdMap map[int32]*fdInfo
}

// newNotifier creates a new notifier object.
func newNotifier() (*notifier, error) {
	kqfd, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}

	w := &notifier{
		kqFD:  kqfd,
		fdMap: make(map[int32]*fdInfo),
	}

	go w.waitAndNotify() // S/R-SAFE: no waiter exists during save / load.

	return w, nil
}

// waitFD waits on mask for fd. The fdMap mutex must be held.
func (n *notifier) waitFD(fd int32, fi *fdInfo, mask waiter.EventMask) error {
	if !fi.waiting && mask == 0 {
		return nil
	}

	// Convert waiter.EventMask to kqueue filter flags
	var readFilter, writeFilter unix.Kevent_t

	// Set up the base filters
	readFilter = unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT,
	}

	writeFilter = unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_WRITE,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT,
	}

	// Determine necessary changes based on current state and requested mask
	switch {
	case !fi.waiting && mask != 0:
		// Adding new filters
		var changes []unix.Kevent_t
		if mask&waiter.ReadableEvents != 0 {
			changes = append(changes, readFilter)
		}
		if mask&waiter.WritableEvents != 0 {
			changes = append(changes, writeFilter)
		}

		if len(changes) > 0 {
			if _, err := unix.Kevent(n.kqFD, changes, nil, nil); err != nil {
				return err
			}
		}
		fi.waiting = true

	case fi.waiting && mask == 0:
		// Removing all filters
		readFilter.Flags = unix.EV_DELETE
		writeFilter.Flags = unix.EV_DELETE

		// Ignore errors here, as the FD might have been closed already
		unix.Kevent(n.kqFD, []unix.Kevent_t{readFilter, writeFilter}, nil, nil)
		fi.waiting = false

	case fi.waiting && mask != 0:
		// Modifying filters
		var changes []unix.Kevent_t

		// First delete existing filters
		readFilterDel := readFilter
		readFilterDel.Flags = unix.EV_DELETE
		writeFilterDel := writeFilter
		writeFilterDel.Flags = unix.EV_DELETE

		// Ignore errors here as some filters might not exist
		unix.Kevent(n.kqFD, []unix.Kevent_t{readFilterDel, writeFilterDel}, nil, nil)

		// Then add the requested ones
		if mask&waiter.ReadableEvents != 0 {
			changes = append(changes, readFilter)
		}
		if mask&waiter.WritableEvents != 0 {
			changes = append(changes, writeFilter)
		}

		if len(changes) > 0 {
			if _, err := unix.Kevent(n.kqFD, changes, nil, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// addFD adds an FD to the list of FDs observed by n.
func (n *notifier) addFD(fd int32, queue *waiter.Queue) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Panic if we're already notifying on this FD.
	if _, ok := n.fdMap[fd]; ok {
		panic(fmt.Sprintf("File descriptor %v added twice", fd))
	}

	info := &fdInfo{queue: queue}
	// We might already have something in queue to wait for.
	if err := n.waitFD(fd, info, queue.Events()); err != nil {
		return err
	}
	// Add it to the map.
	n.fdMap[fd] = info
	return nil
}

// updateFD updates the set of events the fd needs to be notified on.
func (n *notifier) updateFD(fd int32) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if fi, ok := n.fdMap[fd]; ok {
		return n.waitFD(fd, fi, fi.queue.Events())
	}

	return nil
}

// removeFD removes an FD from the list of FDs observed by n.
func (n *notifier) removeFD(fd int32) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Remove from map, then from kqueue object.
	if fi, ok := n.fdMap[fd]; ok {
		n.waitFD(fd, fi, 0)
		delete(n.fdMap, fd)
	}
}

// hasFD returns true if the fd is in the list of observed FDs.
func (n *notifier) hasFD(fd int32) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, ok := n.fdMap[fd]
	return ok
}

// eventMaskFromKqueueFilter converts kqueue filter flags to waiter.EventMask.
func eventMaskFromKqueueFilter(filter int16) waiter.EventMask {
	switch filter {
	case unix.EVFILT_READ:
		return waiter.ReadableEvents
	case unix.EVFILT_WRITE:
		return waiter.WritableEvents
	default:
		return 0
	}
}

// waitAndNotify runs in its own goroutine and loops waiting for io event
// notifications from the kqueue object. Once notifications arrive, they are
// dispatched to the registered queue.
func (n *notifier) waitAndNotify() error {
	// Create a buffer for kqueue events
	events := make([]unix.Kevent_t, 100)

	for {
		// Wait for events with no timeout
		numEvents, err := unix.Kevent(n.kqFD, nil, events, nil)
		if err == unix.EINTR {
			continue
		}

		if err != nil {
			return err
		}

		notified := false
		n.mu.Lock()
		for i := 0; i < numEvents; i++ {
			fd := int32(events[i].Ident)
			if fi, ok := n.fdMap[fd]; ok {
				eventMask := eventMaskFromKqueueFilter(events[i].Filter)
				fi.queue.Notify(eventMask)
				notified = true

				// Since we use EV_ONESHOT, we need to re-add the filter for next time
				n.waitFD(fd, fi, fi.queue.Events())
			}
		}
		n.mu.Unlock()

		if notified {
			// Let goroutines woken by Notify get a chance to run before we
			// kevent again.
			sync.Goyield()
		}
	}
}

var shared struct {
	notifier *notifier
	once     sync.Once
	initErr  error
}

// AddFD adds an FD to the list of observed FDs.
func AddFD(fd int32, queue *waiter.Queue) error {
	shared.once.Do(func() {
		shared.notifier, shared.initErr = newNotifier()
	})

	if shared.initErr != nil {
		return shared.initErr
	}

	return shared.notifier.addFD(fd, queue)
}

// UpdateFD updates the set of events the fd needs to be notified on.
func UpdateFD(fd int32) error {
	return shared.notifier.updateFD(fd)
}

// RemoveFD removes an FD from the list of observed FDs.
func RemoveFD(fd int32) {
	shared.notifier.removeFD(fd)
}

// HasFD returns true if the FD is in the list of observed FDs.
//
// This should only be used by tests to assert that FDs are correctly registered.
func HasFD(fd int32) bool {
	return shared.notifier.hasFD(fd)
}

// NonBlockingPoll polls the given FD in non-blocking fashion. It is used just
// to query the FD's current state.
func NonBlockingPoll(fd int32, mask waiter.EventMask) waiter.EventMask {
	// Create a kqueue to check fd status
	kq, err := unix.Kqueue()
	if err != nil {
		// Conservatively return the mask on error
		return mask
	}
	defer unix.Close(kq)

	var result waiter.EventMask

	// Check for readable events if requested
	if mask&waiter.ReadableEvents != 0 {
		kevs := []unix.Kevent_t{
			{
				Ident:  uint64(fd),
				Filter: unix.EVFILT_READ,
				Flags:  unix.EV_ADD | unix.EV_ONESHOT,
			},
		}

		// Use a zero timeout for non-blocking poll
		ts := unix.Timespec{}

		n, err := unix.Kevent(kq, kevs, kevs, &ts)
		if err == nil && n > 0 {
			result |= waiter.ReadableEvents
		}
	}

	// Check for writable events if requested
	if mask&waiter.WritableEvents != 0 {
		kevs := []unix.Kevent_t{
			{
				Ident:  uint64(fd),
				Filter: unix.EVFILT_WRITE,
				Flags:  unix.EV_ADD | unix.EV_ONESHOT,
			},
		}

		// Use a zero timeout for non-blocking poll
		ts := unix.Timespec{}

		n, err := unix.Kevent(kq, kevs, kevs, &ts)
		if err == nil && n > 0 {
			result |= waiter.WritableEvents
		}
	}

	return result
}
