FROM python:3.9-alpine AS base

# Every challenge must place files into this directory
RUN mkdir /challenge

FROM base AS challenge

ARG FLAG

# The "flag" field in this json object is mandatory for cmgr build process.
RUN echo "{\"flag\":\"$FLAG\"}" > /challenge/metadata.json

# Write the flag to a simple text file that our python server can read.
RUN echo $FLAG > /challenge/flag.txt

# Copy our server
COPY server.py /app/server.py

CMD ["python3", "/app/server.py"]

EXPOSE 8000
# PUBLISH 8000 AS server
