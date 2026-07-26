-- Mochi 数据库表/字段注释
USE mochi;

-- ============================================================
-- users 用户账号及偏好设置
-- ============================================================
ALTER TABLE users COMMENT='用户账号及偏好设置';
ALTER TABLE users
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN email VARCHAR(255) COMMENT '登录邮箱，唯一',
  MODIFY COLUMN password VARCHAR(255) COMMENT 'bcrypt 哈希密码',
  MODIFY COLUMN created_at DATETIME(3) COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) COMMENT '更新时间',
  MODIFY COLUMN proactive_enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否开启主动关怀推送',
  MODIFY COLUMN quiet_hours_start INT NOT NULL DEFAULT 23 COMMENT '免打扰开始小时(0-23)',
  MODIFY COLUMN quiet_hours_end INT NOT NULL DEFAULT 8 COMMENT '免打扰结束小时(0-23)',
  MODIFY COLUMN morning_greeting TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否开启早安问候',
  MODIFY COLUMN reminder_voice TINYINT(1) NOT NULL DEFAULT 1 COMMENT '提醒是否使用语音',
  MODIFY COLUMN follow_up_enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否开启话题跟进',
  MODIFY COLUMN voice_reply_default TINYINT(1) NOT NULL DEFAULT 1 COMMENT '默认是否语音回复',
  MODIFY COLUMN stt_mode VARCHAR(16) NOT NULL DEFAULT 'auto' COMMENT '语音识别模式: auto/manual/push',
  MODIFY COLUMN wellness_nudges_enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否开启健康关怀提醒',
  MODIFY COLUMN wellness_drink TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否提醒喝水',
  MODIFY COLUMN wellness_meal TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否提醒用餐',
  MODIFY COLUMN wellness_rest TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否提醒休息',
  MODIFY COLUMN lunch_hour INT NOT NULL DEFAULT 12 COMMENT '午餐提醒小时',
  MODIFY COLUMN dinner_hour INT NOT NULL DEFAULT 18 COMMENT '晚餐提醒小时',
  MODIFY COLUMN wellness_daily_max INT NOT NULL DEFAULT 2 COMMENT '每日健康提醒上限',
  MODIFY COLUMN learning_prefs_json JSON COMMENT '学习偏好 JSON(topics/level/pace等)';

-- ============================================================
-- pets 用户宠物实例
-- ============================================================
ALTER TABLE pets COMMENT='用户宠物实例，每用户一只';
ALTER TABLE pets
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN user_id BIGINT UNSIGNED COMMENT '所属用户 ID，唯一',
  MODIFY COLUMN name VARCHAR(64) DEFAULT 'Mochi' COMMENT '宠物昵称',
  MODIFY COLUMN personality_json JSON COMMENT '性格设定 JSON(traits/speech_style/style_notes)',
  MODIFY COLUMN sku_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联 pet_skus.sku_id',
  MODIFY COLUMN species VARCHAR(16) NOT NULL DEFAULT 'cat' COMMENT '物种: cat 等',
  MODIFY COLUMN breed VARCHAR(32) NOT NULL DEFAULT '' COMMENT '品种编码',
  MODIFY COLUMN gender VARCHAR(8) NOT NULL DEFAULT 'female' COMMENT '性别: female/male',
  MODIFY COLUMN born_at DATETIME(3) COMMENT '出生时间',
  MODIFY COLUMN max_age_years FLOAT NOT NULL DEFAULT 18 COMMENT '最大寿命(年)',
  MODIFY COLUMN life_stage VARCHAR(16) NOT NULL DEFAULT 'newborn' COMMENT '生命阶段: newborn/child/adult/senior',
  MODIFY COLUMN is_alive TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否存活',
  MODIFY COLUMN created_at DATETIME(3) COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) COMMENT '更新时间';

-- ============================================================
-- chat_messages 聊天消息
-- ============================================================
ALTER TABLE chat_messages COMMENT='人宠聊天消息记录';
ALTER TABLE chat_messages
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN pet_id BIGINT UNSIGNED COMMENT '所属宠物 ID',
  MODIFY COLUMN role ENUM('user','assistant') COMMENT '消息角色: user=用户, assistant=宠物',
  MODIFY COLUMN content TEXT COMMENT '消息正文',
  MODIFY COLUMN created_at DATETIME(3) COMMENT '发送时间';

-- ============================================================
-- memories 宠物记忆
-- ============================================================
ALTER TABLE memories COMMENT='宠物长期/事件/关系记忆，供 RAG 检索';
ALTER TABLE memories
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN pet_id BIGINT UNSIGNED COMMENT '所属宠物 ID',
  MODIFY COLUMN type VARCHAR(16) NOT NULL DEFAULT 'long' COMMENT '记忆类型: long/event/relation',
  MODIFY COLUMN content VARCHAR(1024) COMMENT '记忆内容',
  MODIFY COLUMN importance FLOAT DEFAULT 0.5 COMMENT '重要度 0-1',
  MODIFY COLUMN created_at DATETIME(3) COMMENT '创建时间';

-- ============================================================
-- bond_profiles 人宠羁绊档案
-- ============================================================
ALTER TABLE bond_profiles COMMENT='人宠羁绊档案(亲密度、信任、梗、连续聊天天数等)';
ALTER TABLE bond_profiles
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '宠物 ID，主键',
  MODIFY COLUMN rapport_level TINYINT UNSIGNED NOT NULL DEFAULT 20 COMMENT '亲密度 0-100',
  MODIFY COLUMN trust_level TINYINT UNSIGNED NOT NULL DEFAULT 15 COMMENT '信任度 0-100',
  MODIFY COLUMN shared_topics JSON COMMENT '共同话题列表 JSON',
  MODIFY COLUMN nicknames JSON COMMENT '互称昵称 JSON(user_calls_pet/pet_calls_user)',
  MODIFY COLUMN inside_jokes JSON COMMENT '内部梗列表 JSON',
  MODIFY COLUMN last_mood_tag VARCHAR(32) NOT NULL DEFAULT '' COMMENT '最近一次情绪标签',
  MODIFY COLUMN last_intent VARCHAR(32) NOT NULL DEFAULT '' COMMENT '最近一次对话意图',
  MODIFY COLUMN last_mood_at DATETIME(3) COMMENT '最近情绪更新时间',
  MODIFY COLUMN total_turns INT NOT NULL DEFAULT 0 COMMENT '累计对话轮次',
  MODIFY COLUMN last_chat_day VARCHAR(10) NOT NULL DEFAULT '' COMMENT '最近聊天日期 YYYY-MM-DD',
  MODIFY COLUMN streak_days INT NOT NULL DEFAULT 0 COMMENT '连续聊天天数',
  MODIFY COLUMN updated_at DATETIME(3) COMMENT '更新时间';

-- ============================================================
-- life_states 宠物生命状态
-- pet_id 有 FK 约束，若报错需先 SET FOREIGN_KEY_CHECKS=0
-- ============================================================
ALTER TABLE life_states COMMENT='宠物实时生命状态(情绪、需求属性值)';
SET FOREIGN_KEY_CHECKS = 0;
ALTER TABLE life_states
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '宠物 ID，主键',
  MODIFY COLUMN mood TINYINT UNSIGNED DEFAULT 70 COMMENT '心情 0-100',
  MODIFY COLUMN love TINYINT UNSIGNED DEFAULT 60 COMMENT '爱意 0-100',
  MODIFY COLUMN hungry TINYINT UNSIGNED DEFAULT 30 COMMENT '饥饿 0-100',
  MODIFY COLUMN energy TINYINT UNSIGNED DEFAULT 80 COMMENT '精力 0-100',
  MODIFY COLUMN health TINYINT UNSIGNED DEFAULT 90 COMMENT '健康 0-100',
  MODIFY COLUMN sleep TINYINT UNSIGNED DEFAULT 20 COMMENT '困意 0-100',
  MODIFY COLUMN curiosity TINYINT UNSIGNED DEFAULT 50 COMMENT '好奇心 0-100',
  MODIFY COLUMN knowledge TINYINT UNSIGNED DEFAULT 40 COMMENT '知识/成长 0-100',
  MODIFY COLUMN last_interaction DATETIME(3) COMMENT '最后互动时间',
  MODIFY COLUMN updated_at DATETIME(3) COMMENT '更新时间';
SET FOREIGN_KEY_CHECKS = 1;

-- ============================================================
-- user_briefs 用户画像编译摘要
-- ============================================================
ALTER TABLE user_briefs COMMENT='用户画像编译摘要，注入 LLM system prompt';
ALTER TABLE user_briefs
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '宠物 ID，主键',
  MODIFY COLUMN compiled_text VARCHAR(1400) NOT NULL DEFAULT '' COMMENT '编译后的画像文本',
  MODIFY COLUMN compiled_at DATETIME(3) COMMENT '最近编译时间',
  MODIFY COLUMN char_budget SMALLINT UNSIGNED NOT NULL DEFAULT 1400 COMMENT '字符预算上限',
  MODIFY COLUMN updated_at DATETIME(3) NOT NULL COMMENT '更新时间';

-- ============================================================
-- user_brief_entries 用户画像原始条目
-- ============================================================
ALTER TABLE user_brief_entries COMMENT='用户画像原始条目，编译前逐条存储';
ALTER TABLE user_brief_entries
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '所属宠物 ID',
  MODIFY COLUMN category VARCHAR(16) NOT NULL COMMENT '类别: identity/preference/habit/goal 等',
  MODIFY COLUMN content VARCHAR(256) NOT NULL COMMENT '条目内容',
  MODIFY COLUMN importance FLOAT NOT NULL DEFAULT 0.5 COMMENT '重要度 0-1',
  MODIFY COLUMN source VARCHAR(16) NOT NULL DEFAULT 'extract' COMMENT '来源: extract/manual/chat',
  MODIFY COLUMN status VARCHAR(16) NOT NULL DEFAULT 'approved' COMMENT '状态: approved/pending/rejected',
  MODIFY COLUMN created_at DATETIME(3) NOT NULL COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) NOT NULL COMMENT '更新时间';

-- ============================================================
-- pet_skus 宠物 SKU 商品目录
-- ============================================================
ALTER TABLE pet_skus COMMENT='宠物 SKU 商品目录(品种、定价、外观预设)';
ALTER TABLE pet_skus
  MODIFY COLUMN sku_id VARCHAR(64) NOT NULL COMMENT 'SKU 编码，主键',
  MODIFY COLUMN name VARCHAR(64) NOT NULL COMMENT '商品名称',
  MODIFY COLUMN species VARCHAR(16) NOT NULL DEFAULT 'cat' COMMENT '物种',
  MODIFY COLUMN breed VARCHAR(32) NOT NULL DEFAULT '' COMMENT '品种编码',
  MODIFY COLUMN breed_name VARCHAR(64) NOT NULL DEFAULT '' COMMENT '品种显示名',
  MODIFY COLUMN tier VARCHAR(16) NOT NULL DEFAULT 'standard' COMMENT '档位: standard/premium 等',
  MODIFY COLUMN max_age_years FLOAT NOT NULL DEFAULT 18 COMMENT '最大寿命(年)',
  MODIFY COLUMN price_cny INT NOT NULL DEFAULT 0 COMMENT '价格(分)',
  MODIFY COLUMN tagline VARCHAR(128) NOT NULL DEFAULT '' COMMENT '宣传语',
  MODIFY COLUMN skin_json JSON NOT NULL COMMENT '外观皮肤 JSON',
  MODIFY COLUMN personality_json JSON NOT NULL COMMENT '默认性格预设 JSON',
  MODIFY COLUMN sort_order INT NOT NULL DEFAULT 0 COMMENT '排序权重',
  MODIFY COLUMN enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否上架',
  MODIFY COLUMN created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间';

-- ============================================================
-- pet_orders 宠物订阅/购买订单
-- ============================================================
ALTER TABLE pet_orders COMMENT='宠物订阅/购买订单';
ALTER TABLE pet_orders
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN user_id BIGINT UNSIGNED NOT NULL COMMENT '下单用户 ID',
  MODIFY COLUMN sku_id VARCHAR(64) NOT NULL COMMENT '购买的 SKU',
  MODIFY COLUMN status VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT '状态: pending/paid/claimed/cancelled',
  MODIFY COLUMN personality_json JSON COMMENT '下单时自定义性格 JSON',
  MODIFY COLUMN pet_id BIGINT UNSIGNED COMMENT '认领后关联的宠物 ID',
  MODIFY COLUMN paid_at DATETIME(3) COMMENT '支付时间',
  MODIFY COLUMN claimed_at DATETIME(3) COMMENT '认领/孵化时间',
  MODIFY COLUMN created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间';

-- ============================================================
-- reminders 提醒事项
-- ============================================================
ALTER TABLE reminders COMMENT='提醒事项(聊天创建或手动添加)';
ALTER TABLE reminders
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '关联宠物 ID',
  MODIFY COLUMN user_id BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
  MODIFY COLUMN title VARCHAR(256) NOT NULL COMMENT '提醒标题',
  MODIFY COLUMN fire_at DATETIME(3) NOT NULL COMMENT '触发时间',
  MODIFY COLUMN repeat_rule VARCHAR(32) COMMENT '重复规则: daily/weekly 等',
  MODIFY COLUMN status VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT '状态: pending/fired/cancelled',
  MODIFY COLUMN source VARCHAR(16) NOT NULL DEFAULT 'chat' COMMENT '来源: chat/manual',
  MODIFY COLUMN source_msg VARCHAR(512) COMMENT '来源聊天原文片段',
  MODIFY COLUMN fired_at DATETIME(3) COMMENT '实际触发时间',
  MODIFY COLUMN created_at DATETIME(3) NOT NULL COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) NOT NULL COMMENT '更新时间';

-- ============================================================
-- todos 待办事项
-- ============================================================
ALTER TABLE todos COMMENT='待办事项清单';
ALTER TABLE todos
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '关联宠物 ID',
  MODIFY COLUMN user_id BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
  MODIFY COLUMN title VARCHAR(256) NOT NULL COMMENT '待办标题',
  MODIFY COLUMN due_at DATETIME(3) COMMENT '截止时间',
  MODIFY COLUMN done TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否已完成',
  MODIFY COLUMN sort_order INT NOT NULL DEFAULT 0 COMMENT '排序权重',
  MODIFY COLUMN created_at DATETIME(3) NOT NULL COMMENT '创建时间',
  MODIFY COLUMN updated_at DATETIME(3) NOT NULL COMMENT '更新时间';

-- ============================================================
-- wellness_nudge_logs 健康关怀推送日志
-- ============================================================
ALTER TABLE wellness_nudge_logs COMMENT='健康关怀推送日志(喝水/用餐/休息)';
ALTER TABLE wellness_nudge_logs
  MODIFY COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  MODIFY COLUMN pet_id BIGINT UNSIGNED NOT NULL COMMENT '关联宠物 ID',
  MODIFY COLUMN user_id BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
  MODIFY COLUMN type VARCHAR(32) NOT NULL COMMENT '类型: drink/meal/rest',
  MODIFY COLUMN message VARCHAR(512) NOT NULL COMMENT '推送消息内容',
  MODIFY COLUMN sent_at DATETIME(3) NOT NULL COMMENT '发送时间',
  MODIFY COLUMN created_at DATETIME(3) COMMENT '记录创建时间';
