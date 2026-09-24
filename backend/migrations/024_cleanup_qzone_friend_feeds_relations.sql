-- Clean up artificial friend_visible relations created by migration 023
DELETE FROM relation_events 
WHERE action_type = 'friend_visible' 
  AND context_type = 'qzone_friend';
