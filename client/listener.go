package client

import (
	"errors"
	"runtime/debug"

	"github.com/RomiChan/protobuf/proto"

	eventConverter "github.com/LagrangeDev/LagrangeGo/client/event"
	"github.com/LagrangeDev/LagrangeGo/client/internal/network"
	"github.com/LagrangeDev/LagrangeGo/client/packets/pb/message"
	msgConverter "github.com/LagrangeDev/LagrangeGo/message"
	"github.com/LagrangeDev/LagrangeGo/utils"
	"github.com/LagrangeDev/LagrangeGo/utils/binary"
)

// decoders https://github.com/Mrs4s/MiraiGo/blob/54bdd873e3fed9fe1c944918924674dacec5ac76/client/client.go#L150
var decoders = map[string]func(*QQClient, *network.Packet) (any, error){
	"trpc.msg.olpush.OlPushService.MsgPush":            decodeOlPushServicePacket,
	"trpc.qq_new_tech.status_svc.StatusService.KickNT": decodeKickNTPacket,
}

func decodeOlPushServicePacket(c *QQClient, pkt *network.Packet) (any, error) {
	msg := message.PushMsg{}
	err := proto.Unmarshal(pkt.Payload, &msg)
	if err != nil {
		return nil, err
	}
	pkg := msg.Message
	typ := pkg.ContentHead.Type
	defer func() {
		if r := recover(); r != nil {
			c.error("recovered from panic: %v\n%s", r, debug.Stack())
			c.error("protobuf data: %x", pkt.Payload)
		}
	}()
	if pkg.Body == nil {
		return nil, errors.New("message body is empty")
	}

	switch typ {
	case 166, 208: // 166 for private msg, 208 for private record
		prvMsg := msgConverter.ParsePrivateMessage(&msg)
		_ = c.PreProcessPrivateMessageEvent(prvMsg)
		if prvMsg.Sender.Uin != c.Uin {
			prvMsg.Sender.Nickname = c.cache.GetFriend(prvMsg.Sender.Uin).Nickname
			c.PrivateMessageEvent.dispatch(c, prvMsg)
		} else {
			c.SelfPrivateMessageEvent.dispatch(c, prvMsg)
		}
		return nil, nil
	case 82: // group msg
		grpMsg := msgConverter.ParseGroupMessage(&msg)
		_ = c.PreProcessGroupMessageEvent(grpMsg)
		if grpMsg.Sender.Uin != c.Uin {
			c.GroupMessageEvent.dispatch(c, grpMsg)
		} else {
			c.SelfGroupMessageEvent.dispatch(c, grpMsg)
		}
		return nil, nil
	case 141: // temp msg
		tempMsg := msgConverter.ParseTempMessage(&msg)
		if tempMsg.Sender.Uin != c.Uin {
			c.TempMessageEvent.dispatch(c, tempMsg)
		} else {
			c.SelfTempMessageEvent.dispatch(c, tempMsg)
		}
		return nil, nil
	case 33: // member increase
		pb := message.GroupChange{}
		err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
		if err != nil {
			return nil, err
		}
		c.GroupMemberJoinEvent.dispatch(c, eventConverter.ParseMemberIncreaseEvent(&pb))
		return nil, nil
	case 34: // member decrease
		pb := message.GroupChange{}
		err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
		if err != nil {
			return nil, err
		}
		// 3 是bot自身被踢出，Operator字段会是一个protobuf
		if pb.DecreaseType == 3 && pb.Operator != nil {
			Operator := message.OperatorInfo{}
			err = proto.Unmarshal(pb.Operator, &Operator)
			if err != nil {
				return nil, err
			}
			pb.Operator = utils.S2B(Operator.OperatorField1.OperatorUid)
		}
		c.GroupMemberLeaveEvent.dispatch(c, eventConverter.ParseMemberDecreaseEvent(&pb))
		return nil, nil
	case 84: // group request join notice
		pb := message.GroupJoin{}
		err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
		if err != nil {
			return nil, err
		}
		c.GroupMemberJoinRequestEvent.dispatch(c, eventConverter.ParseRequestJoinNotice(&pb))
		return nil, nil
	case 525: // group request invitation notice
		pb := message.GroupInvitation{}
		err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
		if err != nil {
			return nil, err
		}
		c.GroupMemberJoinRequestEvent.dispatch(c, eventConverter.ParseRequestInvitationNotice(&pb))
		return nil, nil
	case 87: // group invite notice
		pb := message.GroupInvite{}
		err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
		if err != nil {
			return nil, err
		}
		c.GroupInvitedEvent.dispatch(c, eventConverter.ParseInviteNotice(&pb))
		return nil, nil
	case 0x210: // friend event, 528
		subType := pkg.ContentHead.SubType.Unwrap()
		switch subType {
		case 35: // friend request notice
			pb := message.FriendRequest{}
			err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
			if err != nil {
				return nil, err
			}
			c.FriendRequestEvent.dispatch(c, eventConverter.ParseFriendRequestNotice(&pb, &msg))
			return nil, nil
		case 138: // friend recall
			pb := message.FriendRecall{}
			err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
			if err != nil {
				return nil, err
			}
			c.FriendRecallEvent.dispatch(c, eventConverter.ParseFriendRecallEvent(&pb))
			return nil, nil
		case 39: // friend rename
			c.debugln("friend rename")
			pb := message.FriendRenameMsg{}
			err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
			if err != nil {
				return nil, err
			}
			c.RenameEvent.dispatch(c, eventConverter.ParseFriendRenameEvent(&pb, c.cache.GetUin(pb.Body.Data.Uid)))
			return nil, nil
		case 29:
			c.debugln("self rename")
			pb := message.SelfRenameMsg{}
			err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
			if err != nil {
				return nil, err
			}
			c.RenameEvent.dispatch(c, eventConverter.ParseSelfRenameEvent(&pb, &c.transport.Sig))
			return nil, nil
		default:
			c.warning("unknown subtype %d of type 0x210, proto data: %x", subType, pkg.Body.MsgContent)
		}
	case 0x2DC: // grp event, 732
		subType := pkg.ContentHead.SubType.Unwrap()
		switch subType {
		case 20: // nudget(grp_id only)
			return nil, nil
		case 17: // recall
			reader := binary.NewReader(pkg.Body.MsgContent)
			_ = reader.ReadU32() // group uin
			_ = reader.ReadU8()  // reserve
			pb := message.NotifyMessageBody{}
			err = proto.Unmarshal(reader.ReadBytesWithLength("u16", false), &pb)
			if err != nil {
				return nil, err
			}
			c.GroupRecallEvent.dispatch(c, eventConverter.ParseGroupRecallEvent(&pb))
			return nil, nil
		case 12: // mute
			pb := message.GroupMute{}
			err = proto.Unmarshal(pkg.Body.MsgContent, &pb)
			if err != nil {
				return nil, err
			}
			c.GroupMuteEvent.dispatch(c, eventConverter.ParseGroupMuteEvent(&pb))
			return nil, nil
		default:
			c.warning("Unsupported group event, subType: %v", subType)
		}
	default:
		c.warning("Unsupported message type: %v", typ)
	}

	return nil, nil
}

func decodeKickNTPacket(c *QQClient, pkt *network.Packet) (any, error) {
	return nil, nil
}
