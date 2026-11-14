# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.409"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.409/sshtermy_darwin_arm64"
      sha256 "efafa0c78cda5395c97ae3a8addd35825b91d3dae8d97faaca3f76c4044596a6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.409/sshtermy_darwin_amd64"
      sha256 "1bc9db0916b719b10c07de049db572fbef32404a594bd888d3c02d6897922334"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.409/sshtermy_linux_arm64"
      sha256 "a6031dcf38db38e2f8c1e83d7a9dbd5f466d325c37c3edb4684c058481c20770"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.409/sshtermy_linux_amd64"
      sha256 "08283cb33961b0b1f5ea84e2fadcc9a35a499ce5b841ff7262306e2873ddaf00"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
