package bot

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// routeCommand dispatches a command (without the prefix) and returns the
// reply text; empty means no reply.
func (s *Service) routeCommand(ctx context.Context, inst Instance, msg message, command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return inst.MeowSound
	}
	cmd := strings.ToLower(fields[0])
	args := fields[1:]
	switch cmd {
	case "帮助", "help", "菜单":
		return s.helpText(inst)
	case "喵", "meow", "老吴":
		return inst.MeowSound + " 喵喵！"
	case "查人", "人", "who":
		return s.queryPerson(ctx, inst, args)
	case "关系", "path", "rel":
		return s.queryRelation(ctx, inst, args)
	case "查群", "群", "group":
		return s.queryGroup(ctx, inst, args)
	case "掷骰子", "骰子", "dice":
		return fmt.Sprintf("%s 掷出了 %d 点喵！", inst.MeowSound, 1+s.nrand(6))
	case "夸夸", "好运", "lucky":
		return s.entertainLine(inst)
	default:
		return "喵？" + inst.MeowSound + " 没听懂这个指令… 输入 /帮助 看看老吴会什么喵~"
	}
}

func (s *Service) helpText(inst Instance) string {
	return strings.Join([]string{
		inst.Name + " 会的指令喵：",
		"/查人 <QQ> —— 查人物档案（昵称/群数/消息数/最后出现）",
		"/关系 <QQ1> <QQ2> —— 查两个人的共同群与互动",
		"/查群 <群号> —— 查群信息与成员数",
		"/喵 —— 叫一声",
		"/掷骰子 —— 掷骰子",
		"/夸夸 —— 随机好运",
		"（娱乐模式开着的时候，随便聊也可能被老吴蹭到喵~）",
	}, "\n")
}

func cleanQQ(raw string) string {
	s := strings.Trim(raw, "@ \t\r\n")
	s = strings.TrimLeft(s, "0")
	return s
}

func (s *Service) queryPerson(ctx context.Context, inst Instance, args []string) string {
	if len(args) < 1 {
		return "喵？给我一个 QQ 号呀，比如 /查人 10000001" + inst.MeowSound
	}
	qq := cleanQQ(args[0])
	if qq == "" {
		return "这个 QQ 号怪怪的喵…"
	}
	var name string
	var lastSeen time.Time
	var groups, messages int
	err := s.DB.QueryRow(ctx, `
		SELECT COALESCE(p.display_name,''), COALESCE(p.last_seen_at, now()),
			(SELECT count(*) FROM group_memberships gm WHERE gm.person_id=p.id),
			(SELECT count(*) FROM messages m WHERE m.sender_id=p.id)
		FROM persons p JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE pi.platform_user_id=$1`, qq).Scan(&name, &lastSeen, &groups, &messages)
	if err != nil {
		return "没找到这个人的资料喵… " + inst.MeowSound
	}
	if name == "" {
		name = "未命名用户"
	}
	return fmt.Sprintf("QQ %s · %s\n共 %d 个群 · 发言 %d 条\n最后出现 %s",
		qq, name, groups, messages, lastSeen.Local().Format("2006-01-02 15:04"))
}

func (s *Service) queryRelation(ctx context.Context, inst Instance, args []string) string {
	if len(args) < 2 {
		return "喵？给我两个 QQ 号，比如 /关系 10000001 10000003"
	}
	a := cleanQQ(args[0])
	b := cleanQQ(args[1])
	if a == "" || b == "" {
		return "QQ 号不对喵…"
	}
	var sharedGroups, interactions int
	err := s.DB.QueryRow(ctx, `
		SELECT
			(SELECT count(DISTINCT gm1.group_id) FROM group_memberships gm1
				JOIN group_memberships gm2 ON gm2.group_id=gm1.group_id
				JOIN person_identifiers pi1 ON pi1.person_id=gm1.person_id AND pi1.platform='qq' AND pi1.platform_user_id=$1
				JOIN person_identifiers pi2 ON pi2.person_id=gm2.person_id AND pi2.platform='qq' AND pi2.platform_user_id=$2),
			(SELECT count(*) FROM relation_events re
				JOIN person_identifiers a ON a.person_id=re.actor_person_id AND a.platform='qq'
				JOIN person_identifiers t ON t.person_id=re.target_person_id AND t.platform='qq'
				WHERE (a.platform_user_id=$1 AND t.platform_user_id=$2) OR (a.platform_user_id=$2 AND t.platform_user_id=$1))`,
		a, b).Scan(&sharedGroups, &interactions)
	if err != nil {
		return "查关系失败了喵… 可能有人不在库里"
	}
	if sharedGroups == 0 && interactions == 0 {
		return fmt.Sprintf("%s 和 %s 目前没发现直接关联喵。", a, b)
	}
	return fmt.Sprintf("%s ↔ %s\n共同群 %d 个 · 互动事件 %d 次", a, b, sharedGroups, interactions)
}

func (s *Service) queryGroup(ctx context.Context, inst Instance, args []string) string {
	if len(args) < 1 {
		return "喵？给我群号呀，比如 /查群 10000005"
	}
	groupID := strings.TrimSpace(args[0])
	var name string
	var members int
	err := s.DB.QueryRow(ctx, `
		SELECT COALESCE(g.group_name,''), (SELECT count(*) FROM group_memberships gm WHERE gm.group_id=g.id)
		FROM "groups" g WHERE g.platform_group_id=$1`, groupID).Scan(&name, &members)
	if err != nil {
		return "没找到这个群喵… " + inst.MeowSound
	}
	if name == "" {
		name = "未命名群"
	}
	return fmt.Sprintf("群 %s · %s · 成员 %d 人", groupID, name, members)
}
