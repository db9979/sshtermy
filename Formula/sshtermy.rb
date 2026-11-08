# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.257"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.257/sshtermy_darwin_arm64"
      sha256 "05bb3205f44009ba8d19e284bd3dd3e2d25e8b59560354c4c2e20d0b22aa3195"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.257/sshtermy_darwin_amd64"
      sha256 "a0b3543f09defbe126b4d2238d987a45446231af311eba5a5cad40416b9544a7"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.257/sshtermy_linux_arm64"
      sha256 "a10e769bc3ac5061a2b4d53372881ce6930599f1e12e8e015a6892b8c7ca1fb8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.257/sshtermy_linux_amd64"
      sha256 "81f4ac69463c16773caa7ac6b7b914b95d2ddc9206d0c7df06012e0e422a357b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
