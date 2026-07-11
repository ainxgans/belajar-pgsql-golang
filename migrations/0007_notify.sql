CREATE OR REPLACE FUNCTION notify_order() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('orders', json_build_object(
    'id', NEW.id, 'status', NEW.status, 'total', NEW.total)::text);
  RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_notify_order AFTER INSERT ON orders
  FOR EACH ROW EXECUTE FUNCTION notify_order();
