# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.328"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.328/sshtermy_darwin_arm64"
      sha256 "d10dbc34f1581b8371f006e3bbc10fdef8066b140307b0e5defab7db3d15cea1"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.328/sshtermy_darwin_amd64"
      sha256 "aad0ca1d4321d2f517500e968bed6def28b897d91992c336cf16fddcddf71bdd"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.328/sshtermy_linux_arm64"
      sha256 "f46bcaeccbc5de61181ea05ebee139526acbe0dd2db5a0961388be32cbca38e0"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.328/sshtermy_linux_amd64"
      sha256 "b3d3f6b845a4c8aa5b47e17ce7582d9647513bc5366cc56f5fb9adb4dfd7d958"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
