# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.394"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.394/sshtermy_darwin_arm64"
      sha256 "b21126f4d59d490bae0a320ef453e2264b71c4254010ad3dbeb4ca4ca6ff0a39"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.394/sshtermy_darwin_amd64"
      sha256 "1c0a0e6390fddb15f82aee109054a9475388738fefd2237b273e02231bd4332f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.394/sshtermy_linux_arm64"
      sha256 "4189b579ffcbdf30fbbb211b9fa20390129f1ec4f3766784ab0d310a6adbf14c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.394/sshtermy_linux_amd64"
      sha256 "36c4a4a173084d5c01661567b5bedadc2beda87e32d623bb8b35596e2f004c15"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
