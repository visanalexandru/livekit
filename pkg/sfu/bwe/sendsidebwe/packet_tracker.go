// Copyright 2023 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sendsidebwe

import (
	"math/rand"
	"sync"

	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils/mono"
)

// -------------------------------------------------------------------------------

type packetTrackerParams struct {
	Logger logger.Logger
}

type packetTracker struct {
	params packetTrackerParams

	lock sync.Mutex

	sequenceNumber uint64

	baseSendTime int64
	packetInfos  [2048]packetInfo

	baseRecvTime int64
	piLastRecv   *packetInfo
}

func newPacketTracker(params packetTrackerParams) *packetTracker {
	return &packetTracker{
		params:         params,
		sequenceNumber: uint64(rand.Intn(1<<14)) + uint64(1<<15), // a random number in third quartile of sequence number space
	}
}

// SSBWE-TODO: this potentially needs to take isProbe as argument?
func (p *packetTracker) RecordPacketSendAndGetSequenceNumber(atMicro int64, size int, isRTX bool) uint16 {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.baseSendTime == 0 {
		p.baseSendTime = atMicro
	}

	pi := p.getPacketInfo(uint16(p.sequenceNumber))
	pi.sequenceNumber = p.sequenceNumber
	pi.sendTime = atMicro - p.baseSendTime
	pi.recvTime = 0
	pi.size = uint16(size)
	pi.isRTX = isRTX
	// SSBWE-REMOVE p.params.Logger.Infow("packet sent", "packetInfo", pi) // SSBWE-REMOVE

	p.sequenceNumber++

	// extreme case of wrap around before receiving any feedback
	if pi == p.piLastRecv {
		p.piLastRecv = nil
	}

	return uint16(pi.sequenceNumber)
}

func (p *packetTracker) BaseSendTimeThreshold(threshold int64) (int64, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.baseSendTime == 0 {
		return 0, false
	}

	return mono.UnixMicro() - p.baseSendTime - threshold, true
}

func (p *packetTracker) RecordPacketIndicationFromRemote(sn uint16, recvTime int64) (piRecv packetInfo, sendDelta, recvDelta int64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	pi := p.getPacketInfoExisting(sn)
	if pi == nil {
		return
	}

	if recvTime == 0 {
		// maybe lost OR already receied but reported lost in a later report
		piRecv = *pi
		return
	}

	if p.baseRecvTime == 0 {
		p.baseRecvTime = recvTime
		p.piLastRecv = pi
	}

	pi.recvTime = recvTime - p.baseRecvTime
	piRecv = *pi
	if p.piLastRecv != nil {
		sendDelta, recvDelta = pi.sendTime-p.piLastRecv.sendTime, pi.recvTime-p.piLastRecv.recvTime
	}
	p.piLastRecv = pi
	return
}

func (p *packetTracker) getPacketInfo(sn uint16) *packetInfo {
	return &p.packetInfos[int(sn)%len(p.packetInfos)]
}

func (p *packetTracker) getPacketInfoExisting(sn uint16) *packetInfo {
	pi := &p.packetInfos[int(sn)%len(p.packetInfos)]
	if uint16(pi.sequenceNumber) == sn {
		return pi
	}

	return nil
}
