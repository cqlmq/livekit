package rtc

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/pkg/errors"

	"github.com/livekit/livekit-server/pkg/logger"
	"github.com/livekit/livekit-server/pkg/utils"
	"github.com/livekit/livekit-server/proto/livekit"
)

type Participant struct {
	id          string
	peerConn    *webrtc.PeerConnection
	sigConn     SignalConnection
	ctx         context.Context
	cancel      context.CancelFunc
	mediaEngine *MediaEngine
	name        string
	state       livekit.ParticipantInfo_State

	lock           sync.RWMutex
	receiverConfig ReceiverConfig
	tracks         []*Track // tracks that the peer is publishing
	once           sync.Once

	// callbacks & handlers
	// OnPeerTrack - remote peer added a mediaTrack
	OnPeerTrack func(*Participant, *Track)
	// OnOffer - offer is ready for remote peer
	OnOffer func(webrtc.SessionDescription)
	// OnIceCandidate - ice candidate discovered for local peer
	OnICECandidate func(c *webrtc.ICECandidateInit)
	OnStateChange  func(p *Participant, oldState livekit.ParticipantInfo_State)
	OnClose        func(*Participant)
}

func NewParticipant(conf *WebRTCConfig, sc SignalConnection, name string) (*Participant, error) {
	me := webrtc.MediaEngine{}
	me.RegisterDefaultCodecs()
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(conf.SettingEngine))
	pc, err := api.NewPeerConnection(conf.Configuration)

	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	participant := &Participant{
		id:             utils.NewGuid(utils.ParticipantPrefix),
		name:           name,
		peerConn:       pc,
		sigConn:        sc,
		ctx:            ctx,
		cancel:         cancel,
		state:          livekit.ParticipantInfo_JOINING,
		lock:           sync.RWMutex{},
		receiverConfig: conf.receiver,
		tracks:         make([]*Track, 0),
		mediaEngine:    &MediaEngine{},
	}
	participant.mediaEngine.RegisterDefaultCodecs()

	log := logger.GetLogger()

	pc.OnTrack(participant.onTrack)

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ci := c.ToJSON()

		// write candidate
		err := sc.WriteResponse(&livekit.SignalResponse{
			Message: &livekit.SignalResponse_Trickle{
				Trickle: &livekit.Trickle{
					Candidate: ci.Candidate,
					// TODO: there are other candidateInit fields that we might want
				},
			},
		})
		if err != nil {
			log.Errorw("could not send trickle", "err", err)
		}

		if participant.OnICECandidate != nil {
			participant.OnICECandidate(&ci)
		}
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		logger.GetLogger().Debugw("ICE connection state changed", "state", state.String())
		if state == webrtc.ICEConnectionStateConnected {
			participant.updateState(livekit.ParticipantInfo_ACTIVE)
		}
	})

	// TODO: handle data channel

	return participant, nil
}

func (p *Participant) ID() string {
	return p.id
}

func (p *Participant) Name() string {
	return p.name
}

func (p *Participant) State() livekit.ParticipantInfo_State {
	return p.state
}

func (p *Participant) ToProto() *livekit.ParticipantInfo {
	return &livekit.ParticipantInfo{
		Id:   p.id,
		Name: p.name,
	}
}

// Answer an offer from remote participant
func (p *Participant) Answer(sdp webrtc.SessionDescription) (answer webrtc.SessionDescription, err error) {
	if p.state == livekit.ParticipantInfo_JOINING {
		p.updateState(livekit.ParticipantInfo_JOINED)
	}

	if err = p.SetRemoteDescription(sdp); err != nil {
		return
	}

	answer, err = p.peerConn.CreateAnswer(nil)
	if err != nil {
		err = errors.Wrap(err, "could not create answer")
		return
	}

	if err = p.peerConn.SetLocalDescription(answer); err != nil {
		err = errors.Wrap(err, "could not set local description")
		return
	}

	// send client the answer
	err = p.sigConn.WriteResponse(&livekit.SignalResponse{
		Message: &livekit.SignalResponse_Answer{
			Answer: ToProtoSessionDescription(answer),
		},
	})
	if err != nil {
		return
	}

	// only set after answered
	p.peerConn.OnNegotiationNeeded(func() {
		logger.GetLogger().Debugw("negotiation needed", "participantId", p.ID())
		offer, err := p.peerConn.CreateOffer(nil)
		if err != nil {
			logger.GetLogger().Errorw("could not create offer", "err", err)
			return
		}

		err = p.peerConn.SetLocalDescription(offer)
		if err != nil {
			logger.GetLogger().Errorw("could not set local description", "err", err)
			return
		}

		logger.GetLogger().Debugw("created new offer", "offer", offer, "onOffer", p.OnOffer)

		logger.GetLogger().Debugw("sending available offer to participant")
		err = p.sigConn.WriteResponse(&livekit.SignalResponse{
			Message: &livekit.SignalResponse_Negotiate{
				Negotiate: ToProtoSessionDescription(offer),
			},
		})
		if err != nil {
			logger.GetLogger().Errorw("could not send offer to peer",
				"err", err)
		}

		if p.OnOffer != nil {
			p.OnOffer(offer)
		}
	})
	return
}

// SetRemoteDescription when receiving an answer from remote
func (p *Participant) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	if err := p.peerConn.SetRemoteDescription(sdp); err != nil {
		return errors.Wrap(err, "could not set remote description")
	}
	return nil
}

// AddICECandidate adds candidates for remote peer
func (p *Participant) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if err := p.peerConn.AddICECandidate(candidate); err != nil {
		return err
	}
	return nil
}

func (p *Participant) Start() {
	p.once.Do(func() {
		go p.rtcpSendWorker()
	})
}

func (p *Participant) Close() error {
	if p.ctx.Err() != nil {
		return p.ctx.Err()
	}
	p.updateState(livekit.ParticipantInfo_DISCONNECTED)
	if p.OnClose != nil {
		p.OnClose(p)
	}
	p.cancel()
	return p.peerConn.Close()
}

// Subscribes otherPeer to all of the tracks
func (p *Participant) AddSubscriber(otherPeer *Participant) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, track := range p.tracks {
		logger.GetLogger().Debugw("subscribing to mediaTrack",
			"srcPeer", p.ID(),
			"dstPeer", otherPeer.ID(),
			"mediaTrack", track.id)
		if err := track.AddSubscriber(otherPeer); err != nil {
			return err
		}
	}
	return nil
}

func (p *Participant) RemoveSubscriber(peerId string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, track := range p.tracks {
		track.RemoveSubscriber(peerId)
	}
}

// signal connection methods
func (p *Participant) SendJoinResponse(otherParticipants []*Participant) error {
	// send Join response
	return p.sigConn.WriteResponse(&livekit.SignalResponse{
		Message: &livekit.SignalResponse_Join{
			Join: &livekit.JoinResponse{
				Participant:       p.ToProto(),
				OtherParticipants: ToProtoParticipants(otherParticipants),
			},
		},
	})
}

func (p *Participant) SendParticipantUpdate(participants []*livekit.ParticipantInfo) error {
	return p.sigConn.WriteResponse(&livekit.SignalResponse{
		Message: &livekit.SignalResponse_Update{
			Update: &livekit.ParticipantUpdate{
				Participants: participants,
			},
		},
	})
}

func (p *Participant) updateState(state livekit.ParticipantInfo_State) {
	if state == p.state {
		return
	}
	oldState := p.state
	p.state = state
	if p.OnStateChange != nil {
		go func() {
			p.OnStateChange(p, oldState)
		}()
	}
}

// when a new mediaTrack is created, creates a Track and adds it to room
func (p *Participant) onTrack(track *webrtc.Track, rtpReceiver *webrtc.RTPReceiver) {
	logger.GetLogger().Debugw("mediaTrack added", "participantId", p.ID(), "mediaTrack", track.Label())

	// create Receiver
	// p.mediaEngine.TCCExt
	receiver := NewReceiver(p.ctx, p.id, rtpReceiver, p.receiverConfig, 0)
	pt := NewTrack(p.ctx, p.id, p.peerConn, track, receiver)

	p.lock.Lock()
	p.tracks = append(p.tracks, pt)
	p.lock.Unlock()

	pt.Start()
	if p.OnPeerTrack != nil {
		// caller should hook up what happens when the peer mediaTrack is available
		go p.OnPeerTrack(p, pt)
	}
}

func (p *Participant) rtcpSendWorker() {
	t := time.NewTicker(time.Second)
	for {
		select {
		case <-t.C:
			pkts := make([]rtcp.Packet, 0)
			p.lock.RLock()
			for _, r := range p.tracks {
				rr, ps := r.receiver.BuildRTCP()
				if rr.SSRC != 0 {
					ps = append(ps, &rtcp.ReceiverReport{
						Reports: []rtcp.ReceptionReport{rr},
					})
				}
				pkts = append(pkts, ps...)
			}
			p.lock.RUnlock()
			if len(pkts) > 0 {
				if err := p.peerConn.WriteRTCP(pkts); err != nil {
					logger.GetLogger().Errorw("error writing RTCP to peer",
						"peer", p.id,
						"err", err,
					)
				}
			}
		case <-p.ctx.Done():
			t.Stop()
			return
		}
	}
}
