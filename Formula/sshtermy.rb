# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.208"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.208/sshtermy_darwin_arm64"
      sha256 "02a3886c8fcdf10890160f34da6bf4ecd1a6aac25c314950a4e8e76410fbadc5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.208/sshtermy_darwin_amd64"
      sha256 "5aecd981aa64d4fb65cc8c573bf99bcab2ae7a09a8e2a96d65f69529cb13a4a0"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.208/sshtermy_linux_arm64"
      sha256 "878ffe7c63d4be39dfb1f584d832ebeeea1af12e6f9f6bb2fd80b5fb99b88f2c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.208/sshtermy_linux_amd64"
      sha256 "1699ed2794fa45dc16765c9792cce0a9af91ebe840cfb197d8056c45010f7f31"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
