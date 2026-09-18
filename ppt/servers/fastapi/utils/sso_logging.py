import logging


class SSOTicketLogFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        # Uvicorn formats access records as (client, method, target, version, status).
        if isinstance(record.args, tuple) and len(record.args) == 5:
            client, method, target, version, status = record.args
            if isinstance(target, str) and target.split("?", 1)[0] == "/api/v1/auth/sso/callback":
                record.args = (client, method, target.split("?", 1)[0], version, status)
        return True
