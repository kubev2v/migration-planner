# ---------------------------
# Stage 1: VDDK Base
# ---------------------------

FROM quay.io/bodnopoz/vmwd:8.0.3 AS vddk

# ---------------------------
# Stage 2: Final Application
# ---------------------------
FROM ruby:3.2-slim

# Install build tools, git, and other dependencies
RUN apt-get update && apt-get install -y \
      build-essential \
      git \
   && rm -rf /var/lib/apt/lists/*

# Some older RubyGems or corporate proxies benefit from IPv4 fallback:
RUN echo ":ipv4_fallback_enabled: true" >> /root/.gemrc

# Set up a working directory
WORKDIR /app

# Copy VDDK from Stage 1 into this final image
# We'll place it under the same path: /opt/vmware-vix-disklib
COPY --from=vddk /opt/vmware-vix-disklib /opt/vmware-vix-disklib

# Make sure the libraries can be found at runtime
ENV VDDK_PATH=/opt/vmware-vix-disklib
ENV LD_LIBRARY_PATH=$VDDK_PATH/lib64:$LD_LIBRARY_PATH

# Clone manageiq-gems-pending
RUN git clone https://github.com/ManageIQ/manageiq-gems-pending.git

# Update RUBYLIB to include manageiq-gems-pending
ENV RUBYLIB="/app/manageiq-gems-pending/lib:${RUBYLIB}"
ENV RUBYLIB="/app/manageiq-gems-pending/lib/gems/pending:${RUBYLIB}"

# Copy your Gemfile and install bundler + dependencies
COPY Gemfile Gemfile
RUN gem install bundler
RUN bundle install

# Copy your script (e.g. service.rb) into /app
COPY service.rb .

# Default command
CMD ["ruby", "service.rb"]
