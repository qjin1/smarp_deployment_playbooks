FROM ubuntu:14.04

# Setup general environment
ENV SHELL bash
ENV WORKON_HOME /usr/bin/app

# Copy needed files
COPY bin/committee ${WORKON_HOME}/

WORKDIR ${WORKON_HOME}/

# Create empty json objects for the script
RUN echo "{}" > ${WORKON_HOME}/rp.json && echo "{}" > ${WORKON_HOME}/vr.json && echo "{}" > ${WORKON_HOME}/sv.json


# Start the script
CMD ["./committee", "2052", "2053"]

# Open ports for the committee
EXPOSE 2052 2053