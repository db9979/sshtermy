# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.221"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.221/sshtermy_darwin_arm64"
      sha256 "81c5cf9e4766025728daf5d9ceecbba0cd7e281847454d064b0271641b4e6f8d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.221/sshtermy_darwin_amd64"
      sha256 "49b890497e224c58dd9882ba48248b07ab7de1039fa5d051da7b49152e82b06d"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.221/sshtermy_linux_arm64"
      sha256 "c82eb04c24600817c290d2d8a9a6b0350bc3c50dfe6251ad5cf15581cdd3a999"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.221/sshtermy_linux_amd64"
      sha256 "155b5dea192e5c5635b2a9eb0a3801a94fbcfa086d4f9ce7652fe7322c8aeade"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
