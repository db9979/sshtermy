# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.135"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.135/sshtermy_darwin_arm64"
      sha256 "9dae5ed2549bd41ae09db592f3aa5902c5c38ad02307e6b63fa8cbf143cad3bf"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.135/sshtermy_darwin_amd64"
      sha256 "9856bee9ec3f40c3fc12732c10736cbc328842d25870052a75b3c3e989512d44"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.135/sshtermy_linux_arm64"
      sha256 "528a4d6402598281c972ac1f4d244ced0e5004cf8ccfb22cb4adf00226c983ba"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.135/sshtermy_linux_amd64"
      sha256 "746e2666c172c61bfe8f9da18cc2123fc4ecc12e3750e087de7603797b740e39"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
