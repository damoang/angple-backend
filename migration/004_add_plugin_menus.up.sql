-- Migration: Add plugin_name column to menus table
-- This allows plugins to automatically register/unregister menus

-- Add plugin_name column to menus table
ALTER TABLE `menus` ADD COLUMN `plugin_name` VARCHAR(50) DEFAULT NULL COMMENT '플러그인 이름 (NULL이면 코어 메뉴)' AFTER `view_level`;

-- Add index for plugin_name
CREATE INDEX `idx_menus_plugin_name` ON `menus` (`plugin_name`);

-- Create composite index for efficient plugin menu filtering
CREATE INDEX `idx_menus_plugin_active` ON `menus` (`plugin_name`, `is_active`);
