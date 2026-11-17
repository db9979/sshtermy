# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.422"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.422/sshtermy_darwin_arm64"
      sha256 "c7ae85b65ff1fd6503f39008d850555aa51f5123b2e333e5d53aaf12cc33aebb"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.422/sshtermy_darwin_amd64"
      sha256 "f26b96ca820bda6ba0c6a63e0de0b19f143ded1e055105b803e79dbd64e23f07"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.422/sshtermy_linux_arm64"
      sha256 "0884e3adbe7aa0cbee4a0d60bafaa0f0277fb3d8faada032d5b2b56c4fcbd72e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.422/sshtermy_linux_amd64"
      sha256 "7537eccee59267f1af03f994498ad8ce8a5fa8d6a570b780dbd4b7330dc4cd90"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
