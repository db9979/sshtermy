# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.400"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.400/sshtermy_darwin_arm64"
      sha256 "50c6ab3d10d34ff770e8afaa29a5472756b725a21525f3c1c54fbf4036d2b2f5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.400/sshtermy_darwin_amd64"
      sha256 "654d7eb937fedcfda471484e1e90e61e4277dcce404ce59bb5649bff84218972"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.400/sshtermy_linux_arm64"
      sha256 "b149904f0dbd3070ab6826c1c39a4b5392ee38d73bca20f8a63ddd98d9bc69a4"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.400/sshtermy_linux_amd64"
      sha256 "e029d152bb518136daf4525927b336f13ee7bd4a0f4658742c45568c997c65b8"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
