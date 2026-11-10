# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.322"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.322/sshtermy_darwin_arm64"
      sha256 "16ac6398cc0dcf992c85a2fb3d1764289ad984272567f254dd6db7ea3c39e813"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.322/sshtermy_darwin_amd64"
      sha256 "8d91b70114701bba5dc2041640076a2c938388fca6589e97cae2c36e21a9a96c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.322/sshtermy_linux_arm64"
      sha256 "bf854c0db79171f68e001d63a94df89958cf6b8634e1d194873bc528ceb4dc74"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.322/sshtermy_linux_amd64"
      sha256 "cb23a54b5d4a0f0f159d83b9687bbf08103b50cf9c4744f2145aea59a9ffcd6f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
