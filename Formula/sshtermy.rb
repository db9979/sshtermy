# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.432"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.432/sshtermy_darwin_arm64"
      sha256 "e223a58e2acbe1a87dfc1375a6ed1151f152f4143b1c4ed6fde3b8f4bf1e9b47"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.432/sshtermy_darwin_amd64"
      sha256 "34bf922301e1d064cca0fe91d7eb56ffcd5eb9f233272c03a4947bf53f2819a2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.432/sshtermy_linux_arm64"
      sha256 "9211a1030f6756ceb464b13367fe98a6cc7fb576588aaeec1aed25c6f0f557ae"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.432/sshtermy_linux_amd64"
      sha256 "602107d8246806ab76b74a447449947830d02a9b7d75a0bad03e14ebb3736ad6"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
