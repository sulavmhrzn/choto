CREATE TABLE IF NOT EXISTS clicks(
    id BIGSERIAL PRIMARY KEY,
    url_id BIGINT REFERENCES urls(id) ON DELETE CASCADE,
    clicked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),    
    ip_address TEXT,
    country_code CHAR(2), 
    user_agent TEXT,
    device_type TEXT, 
    is_bot BOOLEAN DEFAULT FALSE,
    referrer TEXT
);

CREATE INDEX idx_clicks_url_id ON clicks(url_id);
CREATE INDEX idx_clicks_clicked_at ON clicks(clicked_at);