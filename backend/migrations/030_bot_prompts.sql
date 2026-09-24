-- Add full prompt customization columns to bot_instances
ALTER TABLE bot_instances 
ADD COLUMN IF NOT EXISTS context_prompt text NOT NULL DEFAULT '请以符合你人设的口吻进行自然语言回答。当用户需要查询人、群、关系或发言记录时，请主动调用对应的工具；若工具未查到，请萌态说明。',
ADD COLUMN IF NOT EXISTS cmd_enable_reply text NOT NULL DEFAULT '老吴~ 喵！当前{type}【{target}】已加入白名单并开启服务！🐾',
ADD COLUMN IF NOT EXISTS cmd_disable_reply text NOT NULL DEFAULT '老吴~ 喵！当前{type}【{target}】已关闭服务，老吴去睡觉啦~ Zzz',
ADD COLUMN IF NOT EXISTS fallback_reply text NOT NULL DEFAULT '喵呜……老吴脑子转不过来了，歇会儿再问我吧喵~',
ADD COLUMN IF NOT EXISTS max_tool_hops integer NOT NULL DEFAULT 10;
