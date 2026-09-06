"""The chat service: an agent that answers questions about a documented data model.

It reads and writes exclusively through the Go backend's HTTP API. It holds no
database credentials, which is what keeps the graph projection's invariants in
one place and leaves the agent unable to write anything it was not given an
endpoint for.
"""
