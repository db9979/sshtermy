# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.382"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.382/sshtermy_darwin_arm64"
      sha256 "01a464691ed197dc7f37c52a02388858e0f5a66db4c152fb9c3cf15d4c82fca7"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.382/sshtermy_darwin_amd64"
      sha256 "b2b6ee74e911746fd738f885bcb88da40973aecb571ca22a50ac69436d8e67e5"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.382/sshtermy_linux_arm64"
      sha256 "4c3c8d4eb2aca3dbccd0be97955edb3da9e0faedb7babf413d8f43052f328785"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.382/sshtermy_linux_amd64"
      sha256 "29a6b467e947cecd93f3d417c94666526fe59cabaf5d53ca2351e93476ba3245"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
