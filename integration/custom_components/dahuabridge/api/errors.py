from __future__ import annotations


class DahuaBridgeAPIError(Exception):
    """Raised when a bridge API request fails."""

    def __init__(self, message: str, *, status: int | None = None) -> None:
        super().__init__(message)
        self.status = status
