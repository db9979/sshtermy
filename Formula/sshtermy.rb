# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.368"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.368/sshtermy_darwin_arm64"
      sha256 "435a0d9f11d2761825270e2262dd206f428b1d09bfe8201eb98857439633e576"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.368/sshtermy_darwin_amd64"
      sha256 "f606e7b53b534bd66706323bb84175106a82d4aea25d2cb0a6d3407100b6601f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.368/sshtermy_linux_arm64"
      sha256 "9c0536ac1aa2366a5dbfa1d7bf39d284bf35fc9b426de9df697f408e0d8a3b92"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.368/sshtermy_linux_amd64"
      sha256 "59d6fb9c5633053e3d77bb236864806288fad00c1fa2a6f4d6440626c2de7ef9"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
