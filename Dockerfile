FROM sameersbn/ubuntu:14.04.20160121
MAINTAINER sameer@damagehead.com

# Setup general environment
ENV HOME /usr/bin/app
ENV SHELL bash
ENV WORKON_HOME /usr/bin/app

# Copy needed files
COPY bin/committee /usr/bin/app
COPY src/rp.json /usr/bin/app
COPY src/vr.json /usr/bin/app
COPY src/sv.json /usr/bin/app

# Start the script
ENTRYPOINT ["/usr/bin/app/committee"]
CMD["2052" "2053"]

# Open ports for the committee
EXPOSE 2052 2053