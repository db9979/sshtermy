# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.376"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.376/sshtermy_darwin_arm64"
      sha256 "488c72c80a8d1c3de1d24f3352f895af038796915b6eba9bfa596a17f55fad1d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.376/sshtermy_darwin_amd64"
      sha256 "0cee487f9957623a11c2d62b3a7a380d4ce591102b4943325201fd9216aa852d"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.376/sshtermy_linux_arm64"
      sha256 "e352dc6f3a4b7aab117aa6d906fe754b1b74344da8bce29e9a50fdbfdbd1f805"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.376/sshtermy_linux_amd64"
      sha256 "9f0dabfab997e132c062d5a3d0c97d1cf2d5d26dc53ff366f1171b5b45878cd4"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
